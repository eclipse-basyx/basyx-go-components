# MQTT test certificates

These publicly committed keys and certificates are disposable integration-test
fixtures, not deployment credentials. The test CA signs the server and client
certificates; the server certificate covers localhost, 127.0.0.1, and mqtt_it.
They were generated for this fixture with a ten-year validity period. Replace
the entire set together when renewing it; never configure these keys in a real
deployment. The private CA signing key is not part of the repository.
