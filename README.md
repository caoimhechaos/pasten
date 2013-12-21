pasten
======

pasten is a Go implementation of a Cassandra backed pastebin.
It stores its data in cassandra in the most simple way you could think of:

* Paste IDs are implemented as row keys.
* Paste properties such as owner, title, content or syntax are columns.

Multiple pastebins are supported by specifying separate column families, or,
if desired, entirely separate keyspaces.


Requirements
------------

In order to install pasten, you need to install a few prerequisites:

* golang-go
* golang-ancientauth
* golang-doozer-exportedservice
* golang-cassandra

Installation
------------

Once you have these, just run

    go build

 to complete the build process. You will then receive a binary called pasten
 which implements the pastebin service.

To set up the database, use cassandra-cli to connect to a local Cassandra
instance, then run the commands from the cassandra-schema file. Change them
as appropriate (e.g. if you want to use different settings for the keyspace
which is created).

You will need a certificate signed by the authority which your instance of
the Ancient Login Service runs. If you store the certificate as pasten.pem
and the key as pasten.key in DER format in your running directory, you don't
need to specify the key path flags.

Once this is done, just invoke the binary like:

    ./pasten --template-dir=templates --bind="[::]:8888"

Point your browser to http://localhost:8888/ to see if it worked.


Performance
-----------

On a test, 100 serial requests for a single paste took about 1 seconds.
Response times are typically somewhere around 1 millisecond.

Monitoring
----------

Like any good Go program, pasten exports a few variables under
/debug/vars:

* ancientauth-expired-requests: number of redirected requests from AncientAuth
  which have already expired; if this number is very high, it's possible that
  the system clock is not in sync.
* ancientauth-unknown-root-ca: the AncientAuth server signed the request with
  a CA this service doesn't recognize. Is the --cacert path set correctly?
* ancientauth-broken-requests: number of AncientAuth request packages which
  could not be parsed.
* ancientauth-forged-requests: number of AncientAuth authentication claims
  whose signature could not be verified.
* ancientauth-cert-expired: number of AncientAuth authentication claims signed
  by an expired certificate. If you see this, check the  certificate of the
  login server as well as the local clock on the server running pasten.
* ancientauth-unsupported-algos: number of AncientAuth authentication claims
  which could not be verified because they used an unsupported signature
  algorithm. If this number is higher than 0, this might mean that the
  AncientAuth server and client version diverged too far.
* ancientauth-unauthorized-requests: number of requests to add/edit a paste
  which came from users which were not logged in yet and were redirected to
  the login server.
* ancientauth-login-requests: number of times a requestor was sent to the
  login service.
* ancientauth-login-passed: number of requests which successfully logged in an
  user which was not logged in before.
* ancientauth-generic-errors: unspecified type of AncientAuth errors, check
  the logs.
* cassandra-not-found: number of records which Cassandra couldn't find in the
  database.
* cassandra-found: number of times a record was found successfully in the
  Cassandra database.
* cassandra-errors: number of errors, mapped by type, which ocurred talking
  to the Cassandra server.
* num-requests: total number of HTTP requests received by the binary.
* num-notfounds: total number of paste lookups which could not be retrieved as
  they didn't exist.
* num-views: number of times a paste was requested for viewing.
* num-edits: number of times a paste was added or edited.


Roadmap
-------

For future releases, we are planning to add the following features:

Version 1.0 will feature better web design and pastes which can be set to
expire after a given time.

There are also vague plans to add syntax highlighting for different
languages.


BUGS
----

Bugs for this project are tracked using ditz in the source code tree itself.
The current state of bug squashing can be viewed at
http://pasten.ancient-solutions.com/bugtracker/

To report bugs, please send a pull request with the ditz bug added to the
project
[tonnerre/pasten](https://github.com/tonnerre/pasten/) on GitHub.
