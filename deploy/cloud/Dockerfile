FROM ghcr.io/versatile-sh/connector-engine-headless:latest
COPY deploy/cloud/connector-engine.yaml deploy/cloud/adapters.yaml /etc/connector-engine/
USER 0:0
EXPOSE 8080
CMD ["--config", "/etc/connector-engine/connector-engine.yaml", "--adapters", "/etc/connector-engine/adapters.yaml"]
