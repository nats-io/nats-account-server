module github.com/nats-io/jwt/v2

require (
	github.com/nats-io/jwt v0.3.2
	github.com/nats-io/nkeys v0.1.3
	github.com/stretchr/testify v1.4.0
)

replace github.com/nats-io/jwt v0.3.2 => ../

go 1.13
