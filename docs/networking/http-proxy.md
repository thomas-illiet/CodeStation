# Configure the HTTP proxy

The **Configure HTTP proxy** tool prepares VS Code and new terminals to access the network through an enterprise proxy.

## Applied settings

The form requests the proxy URL, username, and password. It configures:

- the VS Code `http.proxy` setting;
- `HTTP_PROXY` and `http_proxy`;
- `HTTPS_PROXY` and `https_proxy`;
- `NO_PROXY` and `no_proxy`.

CodeStation injects these variables into **new terminals**. Existing terminals must be closed and recreated.

## Example

```text
HTTP proxy: http://proxy.internal.example:3128
HTTPS proxy: http://proxy.internal.example:3128
NO_PROXY: localhost,127.0.0.1,.internal.example
```

The password is kept in VS Code secret storage. The visible `http.proxy` setting contains neither the username nor the password.
