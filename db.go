/*
 * (c) 2013, Caoimhe Chaos <caoimhechaos@protonmail.com>,
 *	     Ancient Solutions. All rights reserved.
 *
 * Redistribution and use in source  and binary forms, with or without
 * modification, are permitted  provided that the following conditions
 * are met:
 *
 * * Redistributions of  source code  must retain the  above copyright
 *   notice, this list of conditions and the following disclaimer.
 * * Redistributions in binary form must reproduce the above copyright
 *   notice, this  list of conditions and the  following disclaimer in
 *   the  documentation  and/or  other  materials  provided  with  the
 *   distribution.
 * * Neither  the  name  of  Ancient Solutions  nor  the  name  of its
 *   contributors may  be used to endorse or  promote products derived
 *   from this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
 * "AS IS"  AND ANY EXPRESS  OR IMPLIED WARRANTIES  OF MERCHANTABILITY
 * AND FITNESS  FOR A PARTICULAR  PURPOSE ARE DISCLAIMED. IN  NO EVENT
 * SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT,
 * INDIRECT, INCIDENTAL, SPECIAL,  EXEMPLARY, OR CONSEQUENTIAL DAMAGES
 * (INCLUDING, BUT NOT LIMITED  TO, PROCUREMENT OF SUBSTITUTE GOODS OR
 * SERVICES; LOSS OF USE,  DATA, OR PROFITS; OR BUSINESS INTERRUPTION)
 * HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
 * STRICT  LIABILITY,  OR  TORT  (INCLUDING NEGLIGENCE  OR  OTHERWISE)
 * ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED
 * OF THE POSSIBILITY OF SUCH DAMAGE.
 */

package main

import (
	"crypto/sha256"
	"database/cassandra"
	"encoding/base64"
	"errors"
	"expvar"
	"hash"
	"log"
	"time"
)

type CassandraStore struct {
	client *cassandra.RetryCassandraClient
	corpus string
}

type Paste struct {
	Id         string
	Title      string
	Syntax     string
	Data       string
	User       string
	CsrfToken  string
	CsrfFailed bool
	Time       time.Time
}

var num_notfound *expvar.Int = expvar.NewInt("cassandra-not-found")
var num_errors *expvar.Map = expvar.NewMap("cassandra-errors")
var num_found *expvar.Int = expvar.NewInt("cassandra-found")

/**
 * Set up a new connection to the cassandra server given as servaddr.
 * Returns a new CassandraStore object which can be used to look up
 * and store pastes.
 */
func NewCassandraStore(servaddr, keyspace, corpus string) *CassandraStore {
	var err error
	var client *cassandra.RetryCassandraClient

	client, err = cassandra.NewRetryCassandraClientTimeout(servaddr,
		10*time.Second)
	if err != nil {
		log.Print("Error opening connection to ", servaddr, ": ", err)
		return nil
	}

	ire, err := client.SetKeyspace(keyspace)
	if ire != nil {
		log.Print("Error setting keyspace to ", corpus, ": ", ire.Why)
		return nil
	}
	if err != nil {
		log.Print("Error setting keyspace to ", corpus, ": ", err)
		return nil
	}

	conn := &CassandraStore{
		client: client,
		corpus: corpus,
	}
	return conn
}

/**
 * Create a new paste entry with the contents "data", owned by the given
 * "user". If supported, the "syntax" can be used for formatting.
 */
func (conn *CassandraStore) AddPaste(paste *Paste, user string) (
	string, error) {
	var mmap map[string]map[string][]*cassandra.Mutation
	var mutations []*cassandra.Mutation
	var mutation *cassandra.Mutation
	var col *cassandra.Column
	var ts int64
	var rmd hash.Hash = sha256.New()
	var digest, pasteid string
	var ire *cassandra.InvalidRequestException
	var ue *cassandra.UnavailableException
	var te *cassandra.TimedOutException
	var err error

	paste.Time = time.Now()
	ts = paste.Time.Unix()

	_, err = rmd.Write([]byte(paste.Data))
	if err != nil {
		return "", err
	}

	digest = base64.URLEncoding.EncodeToString(rmd.Sum(nil))
	pasteid = digest[0:7]

	col = cassandra.NewColumn()
	col.Name = []byte("data")
	col.Value = []byte(paste.Data)
	col.Timestamp = ts
	mutation = cassandra.NewMutation()
	mutation.ColumnOrSupercolumn = cassandra.NewColumnOrSuperColumn()
	mutation.ColumnOrSupercolumn.Column = col
	mutations = append(mutations, mutation)

	col = cassandra.NewColumn()
	col.Name = []byte("owner")
	col.Value = []byte(user)
	col.Timestamp = ts
	mutation = cassandra.NewMutation()
	mutation.ColumnOrSupercolumn = cassandra.NewColumnOrSuperColumn()
	mutation.ColumnOrSupercolumn.Column = col
	mutations = append(mutations, mutation)

	if len(paste.Syntax) > 0 {
		col = cassandra.NewColumn()
		col.Name = []byte("syntax")
		col.Value = []byte(paste.Syntax)
		col.Timestamp = ts
		mutation = cassandra.NewMutation()
		mutation.ColumnOrSupercolumn = cassandra.NewColumnOrSuperColumn()
		mutation.ColumnOrSupercolumn.Column = col
	} else {
		// Create a deletion for the syntax column just in case.
		mutation = cassandra.NewMutation()
		mutation.Deletion = cassandra.NewDeletion()
		mutation.Deletion.Timestamp = ts
		mutation.Deletion.Predicate = cassandra.NewSlicePredicate()
		mutation.Deletion.Predicate.ColumnNames = [][]byte{
			[]byte("syntax")}
	}
	mutations = append(mutations, mutation)

	if len(paste.Title) > 0 {
		col = cassandra.NewColumn()
		col.Name = []byte("title")
		col.Value = []byte(paste.Title)
		col.Timestamp = ts
		mutation = cassandra.NewMutation()
		mutation.ColumnOrSupercolumn = cassandra.NewColumnOrSuperColumn()
		mutation.ColumnOrSupercolumn.Column = col
	} else {
		// Create a deletion for the syntax column just in case.
		mutation = cassandra.NewMutation()
		mutation.Deletion = cassandra.NewDeletion()
		mutation.Deletion.Timestamp = ts
		mutation.Deletion.Predicate = cassandra.NewSlicePredicate()
		mutation.Deletion.Predicate.ColumnNames = [][]byte{
			[]byte("title")}
	}
	mutations = append(mutations, mutation)

	mmap = make(map[string]map[string][]*cassandra.Mutation)
	mmap[pasteid] = make(map[string][]*cassandra.Mutation)
	mmap[pasteid][conn.corpus] = mutations

	ire, ue, te, err = conn.client.BatchMutate(mmap,
		cassandra.ConsistencyLevel_ONE)
	if ire != nil {
		log.Println("Invalid request: ", ire.Why)
		num_errors.Add("invalid-request", 1)
		err = errors.New(ire.String())
		return "", err
	}
	if ue != nil {
		log.Println("Unavailable")
		num_errors.Add("unavailable", 1)
		err = errors.New(ue.String())
		return "", err
	}
	if te != nil {
		log.Println("Request to database backend timed out")
		num_errors.Add("timeout", 1)
		err = errors.New(te.String())
		return "", err
	}
	if err != nil {
		log.Println("Generic error: ", err)
		num_errors.Add("os-error", 1)
		err = errors.New(err.Error())
		return "", err
	}

	return pasteid, nil
}

/**
 * Look up the paste with the short ID "shortid".
 */
func (conn *CassandraStore) LookupPaste(shortid string) (
	*Paste, error) {
	var paste *Paste
	var parent *cassandra.ColumnParent
	var cols []*cassandra.ColumnOrSuperColumn
	var predicate *cassandra.SlicePredicate
	var ire *cassandra.InvalidRequestException
	var ue *cassandra.UnavailableException
	var te *cassandra.TimedOutException
	var err error

	parent = cassandra.NewColumnParent()
	parent.ColumnFamily = conn.corpus

	predicate = cassandra.NewSlicePredicate()
	predicate.ColumnNames = [][]byte{[]byte("data"), []byte("syntax"),
		[]byte("title"), []byte("owner")}

	// TODO(caoimhe): read the whole set of rows in one go.
	cols, ire, ue, te, err = conn.client.GetSlice([]byte(shortid), parent,
		predicate, cassandra.ConsistencyLevel_ONE)
	if ire != nil {
		log.Println("Invalid request: ", ire.Why)
		num_errors.Add("invalid-request", 1)
		return nil, errors.New(ire.Why)
	}

	if ue != nil {
		log.Println("Unavailable")
		num_errors.Add("unavailable", 1)
		return nil, errors.New("Unavailable")
	}

	if te != nil {
		log.Println("Request to database backend timed out")
		num_errors.Add("timeout", 1)
		return nil, errors.New("Timed out")
	}

	if err != nil {
		log.Print("Error getting column: ", err.Error(), "\n")
		num_errors.Add("os-error", 1)
		return nil, err
	}

	if len(cols) == 0 {
		return nil, nil
	}

	paste = new(Paste)
	for _, colsupercol := range cols {
		var col *cassandra.Column

		if !colsupercol.IsSetColumn() {
			continue
		}

		col = colsupercol.Column
		if string(col.Name) == "data" {
			paste.Data = string(col.Value)
		} else if string(col.Name) == "syntax" {
			paste.Syntax = string(col.Value)
		} else if string(col.Name) == "title" {
			paste.Title = string(col.Value)
		} else if string(col.Name) == "owner" {
			paste.User = string(col.Value)
		}
		paste.Time = time.Unix(col.Timestamp, 0)
	}

	if len(paste.Data) <= 0 {
		num_notfound.Add(1)
		return nil, nil
	}
	paste.Id = shortid

	num_found.Add(1)
	return paste, nil
}
