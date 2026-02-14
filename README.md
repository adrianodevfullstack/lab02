# lab02

Laboratório 02 - Sistema de consulta de clima por CEP com OpenTelemetry e Zipkin.

## Descrição

Sistema composto por dois microserviços em Go que recebem um CEP brasileiro e retornam a temperatura atual (Celsius, Fahrenheit e Kelvin) da cidade correspondente, com tracing distribuído via OTEL e Zipkin.

- **ServiceA** (porta 8080): Recebe o CEP via POST, valida e encaminha para o ServiceB
- **ServiceB** (porta 8090): Busca localização pelo CEP e consulta a API de clima

## Pré-requisitos

- [Docker](https://docs.docker.com/get-docker/) e [Docker Compose](https://docs.docker.com/compose/install/)
- [Go 1.21+](https://go.dev/dl/) (para execução local)

## Como rodar os serviços

### Opção 1: Com Docker Compose (recomendado)

Na raiz do projeto:

```bash
# Subir todos os serviços
docker compose up -d

# Ou com build (após alterações no código)
docker compose up -d --build
```

Isso irá subir:

| Serviço         | Porta | Descrição                    |
|-----------------|-------|------------------------------|
| ServiceA        | 8080  | API principal (entrada)       |
| ServiceB        | 8090  | Orquestração CEP + clima      |
| Zipkin          | 9411  | UI de tracing                |
| Jaeger          | 16686 | UI de tracing alternativa    |
| Prometheus      | 9090  | Métricas                     |
| Grafana         | 3000  | Dashboards (admin/admin)     |
| OTEL Collector  | 4317  | Coleta de telemetria         |

**Parar os serviços:**

```bash
docker compose down
```

### Opção 2: Localmente (sem Docker)

Para desenvolvimento ou testes:

**1. Inicie o OTEL Collector e Zipkin:**

```bash
docker compose up -d zipkin otel-collector
```

**2. Em um terminal, rode o ServiceB:**

```bash
go run ./ServiceB/
```

**3. Em outro terminal, rode o ServiceA:**

```bash
SERVICE_B_URL=http://localhost:8090 go run ./ServiceA/
```

O ServiceA usa `http://localhost:8090` como padrão quando `SERVICE_B_URL` não está definido.

## Uso da API

### Requisição

```bash
curl -X POST http://localhost:8080/ \
  -H "Content-Type: application/json" \
  -d '{"cep": "29902555"}'
```

### Respostas

**Sucesso (200):**
```json
{
  "city": "Vitória",
  "temp_C": 28.5,
  "temp_F": 83.3,
  "temp_K": 301.65
}
```

**CEP inválido (422):**
```json
{
  "error": "invalid zipcode"
}
```

**CEP não encontrado (404):**
```json
{
  "error": "can not find zipcode"
}
```

## Endpoints úteis

- **ServiceA:** http://localhost:8080/
- **Métricas ServiceA:** http://localhost:8080/metrics
- **Métricas ServiceB:** http://localhost:8090/metrics
- **Zipkin (tracing):** http://localhost:9411
- **Grafana:** http://localhost:3000 (usuário: admin, senha: admin)
