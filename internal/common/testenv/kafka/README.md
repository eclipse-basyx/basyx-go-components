# Kafka integration fixture

Apache Kafka exposes internal plaintext plus host plaintext, SASL, mutual TLS,
and SASL over mutual TLS listeners. The init service creates a three-partition
event topic and credentials for PLAIN, SCRAM-SHA-256, and SCRAM-SHA-512.

The PEM files are public, test-only credentials, valid for ten years from
September 2026. The certificate covers `kafka_it`, `localhost`, and `127.0.0.1`.
`server.pem` contains a PKCS#8 private key and certificate; `client.pem` and
`client-key.pem` contain the same test identity for mutual TLS.
Never use these credentials outside local tests.
