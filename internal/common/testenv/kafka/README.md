# Kafka test certificates

These PEM files are public test credentials used by the Kafka integration
[Compose fixture](docker-compose.yml). Do not use them in deployments.

- `ca.pem`: CA trusted by the test broker and clients.
- `server.pem`: broker certificate and PKCS#8 private key.
- `client.pem` and `client-key.pem`: the same identity, used by mutual TLS tests.

When renewing the certificates, preserve the names `kafka_it`, `localhost`, and
`127.0.0.1`, and allow both server and client authentication. The current
certificates expire in September 2036.
