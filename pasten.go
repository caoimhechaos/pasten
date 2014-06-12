/*
 * (c) 2013, Tonnerre Lombard <tonnerre@ancient-solutions.com>,
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
	"ancient-solutions.com/ancientauth"
	"ancient-solutions.com/doozer/exportedservice"
	"expvar"
	"flag"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var store *CassandraStore
var num_requests *expvar.Int = expvar.NewInt("num-requests")
var num_views *expvar.Int = expvar.NewInt("num-views")
var num_edits *expvar.Int = expvar.NewInt("num-edits")
var num_notfounds *expvar.Int = expvar.NewInt("num-notfounds")

var paste_templ *template.Template
var display_templ *template.Template
var error_templ *template.Template
var fourohfour_templ *template.Template
var authenticator *ancientauth.Authenticator

func Pasten(w http.ResponseWriter, req *http.Request) {
	var pasteid string = strings.Split(req.URL.Path, "/")[1]
	var err error

	num_requests.Add(1)

	if pasteid == "" {
		/* People need to be logged in in order to add URLs. */
		var dest *url.URL = &url.URL{
			Path: "/",
		}
		var user string = authenticator.GetAuthenticatedUser(req)
		var paste Paste

		// TODO(tonnerre): Count errors properly here.
		if user == "" {
			authenticator.RequestAuthorization(w, req)
			return
		}

		err = req.ParseForm()
		if err != nil {
			error_templ.Execute(w, err.Error())
			return
		}

		paste.User = user
		paste.Data = req.FormValue("paste")
		paste.Syntax = req.FormValue("syntax")
		paste.Title = req.FormValue("title")
		paste.CsrfToken, err = authenticator.GenCSRFToken(
			req, dest, 20*time.Minute)
		if err != nil {
			error_templ.Execute(w, err.Error())
			return
		}

		if paste.Data != "" {
			var verified bool

			verified, err = authenticator.VerifyCSRFToken(req,
				req.FormValue("csrftoken"), false)
			if err != nil && err != ancientauth.CSRFToken_WeakProtectionError {
				error_templ.Execute(w, err.Error())
				return
			}

			if verified {
				paste.Id, err = store.AddPaste(&paste, user)
				if err != nil {
					err = error_templ.Execute(w, err.Error())
					if err != nil {
						log.Print("Error executing error template: ", err)
					}
					return
				}
				num_edits.Add(1)
				err = display_templ.Execute(w, paste)
				if err != nil {
					log.Print("Error executing display template: ", err)
				}
				return
			} else {
				paste.CsrfFailed = true
			}
		}

		err = paste_templ.Execute(w, paste)
		if err != nil {
			log.Print("Error executing display template: ", err)
		}
	} else {
		var paste *Paste
		paste, err = store.LookupPaste(pasteid)
		if err != nil {
			err = error_templ.Execute(w, err.Error())
			if err != nil {
				log.Print("Error executing error template: ", err)
			}
			return
		}

		if paste == nil {
			w.WriteHeader(http.StatusNotFound)
			err = fourohfour_templ.Execute(w, pasteid)
			if err != nil {
				log.Print("Error executing 404 template: ", err)
			}
			num_notfounds.Add(1)
		} else {
			err = display_templ.Execute(w, paste)
			if err != nil {
				log.Print("Error executing display template: ", err)
			}
			num_views.Add(1)
		}
	}
}

func main() {
	var help bool
	var cassandra_server, keyspace, corpus string
	var ca, pub, priv, authserver string
	var bindto, templatedir, servicename string
	var doozer_uri, doozer_buri string
	var exporter *exportedservice.ServiceExporter
	var err error

	flag.BoolVar(&help, "help", false, "Display help")
	flag.StringVar(&bindto, "bind", "[::]:80",
		"The address to bind the web server to")
	flag.StringVar(&cassandra_server, "cassandra-server", "localhost:9160",
		"The Cassandra database server to use")
	flag.StringVar(&keyspace, "keyspace", "pasten",
		"The Cassandra keyspace the links are stored in. "+
			"The default should be fine.")
	flag.StringVar(&corpus, "corpus", "pastes",
		"The column family containing the paste data for this service")
	flag.StringVar(&ca, "cacert", "cacert.pem",
		"Path to the X.509 certificate of the certificate authority")
	flag.StringVar(&pub, "cert", "pasten.pem",
		"Path to the X.509 certificate")
	flag.StringVar(&priv, "key", "pasten.key",
		"Path to the X.509 private key file")
	flag.StringVar(&templatedir, "template-dir", "/var/www/templates",
		"Path to the HTML templates for the web interface")
	flag.StringVar(&authserver, "auth-server",
		"login.ancient-solutions.com",
		"The server to send the user to")
	flag.StringVar(&doozer_uri, "doozer-uri", os.Getenv("DOOZER_URI"),
		"Doozer URI to connect to")
	flag.StringVar(&doozer_buri, "doozer-boot-uri",
		os.Getenv("DOOZER_BOOT_URI"),
		"Doozer Boot URI to find named clusters")
	flag.StringVar(&servicename, "exported-name", "",
		"Name to export the service as in Doozer")
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(1)
	}
	paste_templ = template.Must(template.ParseFiles(templatedir +
		"/paste.tmpl"))
	display_templ = template.Must(template.ParseFiles(templatedir +
		"/display.tmpl"))
	error_templ = template.Must(template.ParseFiles(templatedir +
		"/error.tmpl"))
	fourohfour_templ = template.Must(template.ParseFiles(templatedir +
		"/notfound.tmpl"))

	authenticator, err = ancientauth.NewAuthenticator("Paste Bin", pub,
		priv, ca, authserver)
	if err != nil {
		log.Fatal("NewAuthenticator: ", err)
	}

	store = NewCassandraStore(cassandra_server, keyspace, corpus)
	if store == nil {
		os.Exit(2)
	}

	http.Handle("/", http.HandlerFunc(Pasten))
	http.Handle("/css/", http.FileServer(http.Dir(templatedir)))
	http.Handle("/js/", http.FileServer(http.Dir(templatedir)))

	if len(servicename) > 0 {
		exporter, err = exportedservice.NewExporter(doozer_uri,
			doozer_buri)
		if err != nil {
			log.Fatal("NewExporter: ", err)
		}

		err = exporter.ListenAndServeNamedHTTP(servicename, bindto,
			nil)
		if err != nil {
			log.Fatal("ListenAndServeNamedHTTP: ", err)
		}
	} else {
		err = http.ListenAndServe(bindto, nil)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}
}
