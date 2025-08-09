FROM python:3.11-slim

WORKDIR /app

RUN apt-get update && apt-get install -y \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY pyproject.toml ./
RUN pip install --no-cache-dir uv && \
    uv pip install --system --no-cache .

COPY src/ ./src/

RUN useradd --create-home --shell /bin/bash --uid 65534 operator && \
    chown -R operator:operator /app

USER operator

EXPOSE 8080

CMD ["python", "-m", "vault_autounseal_operator.main"]