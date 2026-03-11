FROM scratch

# wasi/http proxy component produced by TinyGo (wasip2-http target).
COPY main.wasm /main.wasm

# Optional: override with container env if needed.
ENV WASMTIME_HTTP_PROXY_SOCKET_ADDR=0.0.0.0:8080

EXPOSE 8080

# The wasmtime shim uses this as the module path/entrypoint.
CMD ["/main.wasm"]