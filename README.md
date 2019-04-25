![NATS](logos/large-logo.png)

# nats-account-server

[![License][License-Image]][License-Url]
[![ReportCard][ReportCard-Image]][ReportCard-Url]
[![Build][Build-Status-Image]][Build-Status-Url]
[![Coverage][Coverage-Image]][Coverage-Url]

A simple HTTP server to host account JWTs for [nats-server 2.0](https://nats.io) account authentication.

NATS 2.0 introduced the concept of accounts to provide secure multi-tenancy through separate subject spaces.
These accounts are configured with JWTs that encapsulate the account settings. User JWTs are used to authenticate.
The nats-server can be configured to use local account information or to rely on an external, HTTP-based source for
account JWTs. The server in this repository is intended as a simple to use solution for hosting account JWTs.

* [HTTP API](#http)
* [JWT Stores](#store)
* [NATS Notifications](#nats)
* [Running the Server](#run)
* [Configuration](#config)
  * [Logging](#logconfig)
  * [TLS](#tlsconfig)
  * [NATS](#natsconfig)
  * [HTTP](#httpconfig)
  * [Store](#storeconfig)
* [Building the Server](#build)
* [External Resources](#resources)
* [License](#license)

This server is also intended as an example for developers that want to implement their own version of the server for resolving
JWTs.

The nats-account-server does not have or need access to any private keys. Public keys are used to identify the operator and accounts.

<a name="http"></a>

## HTTP API

The server's primary responsibility is to host account JWTs over HTTP. The NATS server, or other clients, can access
an accounts JWT using the URL:

```bash
GET /jwt/v1/accounts/<pubkey>
```

this endpoint will:

* Contains cache control headers
* Uses the JTI as the ETag
* Has content type `application/jwt`
* Is unvalidated, and the JWT may have expired
* Returns 304 if the request contains the appropriate If-None-Match header
* Returns 404 if the JWT is not found
* Return 200 and the encoded JWT if it is found

Several optional and mutually exclusive query parameters are supported:

* `text` - set to "true" to change the content type to text/plain
* `decode` - set to "true" to display the decoded JSON for the JWT header and body
* `check` - set to "true" to tell the server to return 404 if the JWT is expired
* `notify` - set to "true" to tell the server to send a [notification](#nats) to the nats-server indicating that this account changed.

For example, `curl http://localhost:8080/jwt/v1/accounts/<pubkey>?check=true` will return a 404 error
if the JWT is expired.

The NATS server will hit this endpoint without a public key on startup to test that the server is available,
so the server responds to `GET /jwt/v1/accounts/` and `GET /jwt/v1/accounts` with a status 200.

When run with a [mutable JWT store](#store), the server will also allow JWTs to be uploaded.

```bash
POST /jwt/v1/accounts/<pubkey>
```

The JWT must be signed by the operator specified in the [server's configuration](#config).

A status 400 is returned if there is a problem with the JWT or the server is in read-only mode. In rare
cases a status 500 may be returned if there was an issue saving the JWT.

<a name="store"></a>

## JWT Stores

This repository provides three JWT store implementations, and can be extended to provide others.

* Directory Store - The directory store saves and loads JWTs into an optionally sharded structure under a root folder. The last two
characters in the accounts public key are used to create a sub-folder, and the accounts public key is used as the file name, with
".jwt" appended. The directory store can be run in read-only mode.

* NSC Store - The NSC store uses an operator folder, as created by the `nsc` tool as a JWT source. The store is read-only, but
will automatically host new JWTs added by `nsc`. Use the `notify` flag to push the change to the nats-servers.

* Memory Store - By default the account server uses an in-memory store. This store is provided for testing and shouldn't be used in
production.

The server understands one special JWT that doesn't have to be in the store. This JWT, called the system account, can be set up in
the [config](#config) file. The server will always try to return a JWT from the store, and if that fails, and the request was for the
system JWT will try to return it directly.

<a name="nats"></a>

## NATS Notifications

The nats-server listens for notifications about changes to account JWTs on a system account. The account-server sends these notifications when a POST request is received, or when the `notify` query parameter is used with a GET request. Security for the NATS connection is configured via a credentials file in the configuration or on the command line.

The account server can be started with or without a NATS configuration, and will try to connect on a regular timer if it is configured to talk to NATS but can't find a server. This reconnect strategy allows us to avoid the chicken and egg problem where the NATS server requires its account resolver to be running but the account server can't find a valid nats-server to connect to.

<a name="run"></a>

## Running the server

The server will compile to an executable named `nats-account-server`. You can run this executable without flags to get a memory based account server with no
[notifications](#nats). For more interesting operation there are a hand full of flags and a [configuration](#config) file.

To run the server on an NSC folder, use the `-nsc` flag to point to a specific operator folder in your NSC directory. For example:

```bash
% nats-account-server -nsc ~/.nsc/nats/signing_test
```

will run the account server, in read-only mode, on the operator folder named *signing_test*. Any account in signing test will be available via HTTP to the nats-server or another client. To enable [notifications](#nats) you can add the `-nats` parameter, and optionally the `-creds` flag to set a credential file.

```bash
% nats-account-server -nsc ~/.nsc/nats/signing_test -nats nats://localhost:4222
```

If the `-nats` flag is set, you can force a notification using a GET request like:

```bash
% curl http://localhost:58385/jwt/v1/accounts/ABVSBM3U45DGYEUECKXQS7BENHWG7KFQUDRTEHAJASORPVWBZ4HOIKCH\?notify\=true
```

The nsc-based account server will not accept POST requests.

To run with a [configuration](#config) file, use the `-c` flag:

```bash
% nats-account-server -c <config file>
```

Any settings in the configuration file are applied first, then other flags are used. This allows you to override some settings on the command line.

Finally, you can use the `-D`, `-V` or `-DV` flags to turn on debug or verbose logging. The `-DV` option will turn on all logging, depending on the config file settings.

<a name="config"></a>

## Configuration

The configuration file uses the same YAML/JSON-like format as the nats-server. Configuration is organized into a root section with several sub-sections. The root section can contain the following entries:

* `logging` - configuration for [server logging](#logconfig)
* `nats` - configuration for the [NATS connection](#natsconfig)
* `http` - configuration for the [HTTP Server](#httpconfig)
* `store` - the [store configuration](#storeconfig) parameters
* `operatorjwtpath` - the path to an operator JWT, required for stores that accept POST request, all JWTs sent in a POST must be signed by
one of the operator's keys
* `systemaccountjwtpath` - the path to an account JWT that should be returned as the system account, works outside the normal store if necessary, however, the system account can be in the store, in which case this setting is optional

The default configuration is:

```yaml
{
    logging: {
        colors: true,
        time:   true,
        debug:  false,
        trace:  false,
    },
    http: {
        readtimeout:  5000,
        writetimeout: 5000,
    },
    nats: {
        connecttimeout: 5000,
        reconnectwait:  1000,
        maxreconnects:  0,
    },
}
```

<a name="logconfig"></a>

### Logging

Logging is configured in a manner similar to the nats-server:

```yaml
logging: {
  time: true,
  debug: false,
  trace: false,
  colors: true,
  pid: false,
}
```

These properties are configured for:

* `time` - include the time in logging statements
* `debug` - include debug logging
* `trace` - include verbose, or trace, logging
* `colors` - colorize the logging statements
* `pid` - include the process id in logging statements

Debug and trace can also be set on the command line with `-D`, `-V` and `-DV` to match the nats-server.

<a name="tlsconfig"></a>

### TLS

The NATS and HTTP configurations take an optional TLS setting. The TLS configuration takes three possible settings:

* `root` - file path to a CA root certificate store, used for NATS connections
* `cert` - file path to a server certificate, used for HTTPS monitoring and optionally for client side certificates with NATS
* `key` - key for the certificate store specified in cert

<a name="natsconfig"></a>

### NATS Configuration

The account server can connect to NATS to send [notifications](#nats) to the nats-servers associated with it. This connection requires a single section in the main configuration:

```yaml
nats: {
  Servers: ["localhost:4222"],
  ConnectTimeout: 5000,
  MaxReconnects: 5,
  ReconnectWait: 5000,
}
```

NATS can be configured with the following properties:

* `servers` - an array of server URLS
* `connecttimeout` - the time, in milliseconds, to wait before failing to connect to the NATS server
* `reconnectwait` - the time, in milliseconds, to wait between reconnect attempts
* `maxreconnects` - the maximum number of reconnects to try before exiting the bridge with an error.
* `tls` - (optional) [TLS configuration](#tlsconfig). If the NATS server uses unverified TLS with a valid certificate, this setting isn't required.
* `UserCredentials` - (optional) the path to a credentials file for connecting to the system account.

The account server uses the reconnect wait in two ways. First, it is used for normal NATS reconnections. Second, it is used with a timer if the account server can't connect to the NATS server upon startup. This failure at startup is expected since the nats-server configured with a URL resolver requires an account-server but the account server doesn't "require" NATS to host JWTs.

<a name="httpconfig"></a>

### HTTP Configuration

HTTP is configured in the main section under `http`:

```yaml
http: {
  host: "localhost",
  port: 9090,
  readtimeout: 5000,
  writetimeout: 5000,
}
```

The HTTP section contains the following properties:

* `host` - a host on the local machine
* `port` - the port to run on
* `readtimeout` - the time, in milliseconds, to wait for reads to complete
* `writetimeout` - the time, in milliseconds, to wait for writes to complete
* `tls` - (optional) [TLS configuration](#tls), only the `cert` and `key` properties are used.

If no host and port are provided the server will bind to all network interfaces and an ephemeral port.

<a name="storeconfig"></a>

### Store Configuration

The store is configured in a single section called `store`:

```yaml
store: {
    nsc: ~/.nsc/nats/signing_test
}
```

This section can contain the following properties:

* `nsc` - the path to an NSC operator folder, this setting takes precedent over the others
* `dir` - the path to a folder to use for storing JWTS
* `readonly` - turns on/off mutability for the directory or memory stores
* `shard` - if "true" the directory store will shard the files into sub-directories based on the last 2 characters of the public keys.

A memory store is created if `nsc` and `dir` are not set.

<a name="build"></a>

## Building the Server

This project uses go modules and provides a make file. You should be able to simply:

```bash
% git clone https://github.com/nats-io/account-server.git
% cd account-server
% make
```

Use `make test` to run the tests, and `make install` to install.

The server does depend on the nats-server repo as well as nsc, and as a result contains a number of dependencies. However, the final executable is fairly small, ~10mb.

<a name="resources"></a>

## External Resources

* [NATS](https://nats.io/documentation/)
* [NATS server](https://github.com/nats-io/gnatsd)

[License-Url]: https://www.apache.org/licenses/LICENSE-2.0
[License-Image]: https://img.shields.io/badge/License-Apache2-blue.svg
[Build-Status-Url]: https://travis-ci.com/nats-io/account-server
[Build-Status-Image]: https://travis-ci.com/nats-io/account-server.svg?branch=master
[Coverage-Url]: https://coveralls.io/r/nats-io/account-server?branch=master
[Coverage-image]: https://coveralls.io/repos/github/nats-io/account-server/badge.svg?branch=master
[ReportCard-Url]: https://goreportcard.com/report/nats-io/account-server
[ReportCard-Image]: https://goreportcard.com/badge/github.com/nats-io/account-server

<a name="license"></a>

## License

Unless otherwise noted, the nats-account-server source files are distributed under the Apache Version 2.0 license found in the LICENSE file.