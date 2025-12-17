# Apache Reverse Proxy Setup

This configuration uses Apache HTTP Server as a reverse proxy to provide DNS-based routing for the BaSyx components.

## Configuration Files

- `httpd.conf` - Main Apache configuration with required modules
- `vhosts.conf` - Virtual host definitions for each service

## Virtual Host Mapping

The following hostnames are configured:

- `aasgui.basyx.localhost` → AAS Web UI (port 3000)
- `aasreg.basyx.localhost` → AAS Registry (port 8080)
- `smreg.basyx.localhost` → Submodel Registry (port 8080)
- `aasenv.basyx.localhost` → AAS Environment (port 8081)
- `discovery.basyx.localhost` → AAS Discovery (port 8081)
- `keycloak.basyx.localhost` → Keycloak Identity Provider (port 8080)

## Usage

The Apache proxy listens on port 80 and routes requests based on the `Host` header to the appropriate backend service.

### Accessing Services

After starting the docker-compose stack, you can access:

- **AAS Web UI**: http://aasgui.basyx.localhost
- **AAS Registry**: http://aasreg.basyx.localhost
- **Submodel Registry**: http://smreg.basyx.localhost
- **AAS Environment**: http://aasenv.basyx.localhost
- **AAS Discovery**: http://discovery.basyx.localhost
- **Keycloak**: http://keycloak.basyx.localhost

### Local DNS Setup

Add the following entries to your `/etc/hosts` file (on macOS/Linux) or `C:\Windows\System32\drivers\etc\hosts` (on Windows):

```
127.0.0.1 aasgui.basyx.localhost
127.0.0.1 aasreg.basyx.localhost
127.0.0.1 smreg.basyx.localhost
127.0.0.1 aasenv.basyx.localhost
127.0.0.1 discovery.basyx.localhost
127.0.0.1 keycloak.basyx.localhost
```

## Notes

- All services communicate internally using the container names
- The proxy preserves the original `Host` header when forwarding requests
- The default virtual host is set to `aasgui.basyx.localhost` (AAS Web UI)
