# Apache Reverse Proxy Setup

This configuration is an optional Apache HTTP Server reverse-proxy sample for DNS-based routing to the BaSyx secured example components. The main `examples/BaSyxSecuredExample/docker-compose.yml` does not start Apache; run an Apache container with these files mounted if you want host-name based access.

## Configuration Files

- `httpd.conf` - Main Apache configuration with required modules
- `vhosts.conf` - Virtual host definitions for each service

## Virtual Host Mapping

The following hostnames are configured:

- `aasgui.basyx.localhost` → AAS Web UI (port 3000)
- `aasreg.basyx.localhost` → AAS Registry (`aas-registry:5004`)
- `smreg.basyx.localhost` → Submodel Registry (`sm-registry:5004`)
- `discovery.basyx.localhost` → AAS Discovery (`aas-discovery:5004`)
- `keycloak.basyx.localhost` → Keycloak Identity Provider (port 8080)

## Usage

The Apache proxy listens on port 80 and routes requests based on the `Host` header to the appropriate backend service.

### Accessing Services

After starting the compose stack and a proxy container using these Apache files, you can access:

- **AAS Web UI**: http://aasgui.basyx.localhost
- **AAS Registry**: http://aasreg.basyx.localhost
- **Submodel Registry**: http://smreg.basyx.localhost
- **AAS Discovery**: http://discovery.basyx.localhost
- **Keycloak**: http://keycloak.basyx.localhost

### Local DNS Setup

Add the following entries to your `/etc/hosts` file (on macOS/Linux) or `C:\Windows\System32\drivers\etc\hosts` (on Windows):

```
127.0.0.1 aasgui.basyx.localhost
127.0.0.1 aasreg.basyx.localhost
127.0.0.1 smreg.basyx.localhost
127.0.0.1 discovery.basyx.localhost
127.0.0.1 keycloak.basyx.localhost
```

## Notes

- All services communicate internally using the container names from `docker-compose.yml`
- The proxy preserves the original `Host` header when forwarding requests
- The default virtual host is set to `aasgui.basyx.localhost` (AAS Web UI)
