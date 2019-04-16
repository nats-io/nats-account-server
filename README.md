# account-server

A simple HTTP/NATS server to host account JWTs for nats-server 2.0 account authentication.

NATS 2.0 introduced the concept of accounts to provide secure multi-tenancy through separate subject spaces.
These accounts are configured with JWTs that encapsulate the account settings. User JWTs are used to authenticate.
The nats-server can be configured to use local account information or to rely on an external, HTTP-based source for
account JWTs. The server in this repository is intended as a simple to use solution for hosting account JWTs.

