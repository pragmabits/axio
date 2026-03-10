# Axio

![Português](https://img.shields.io/badge/lang-pt--BR-green.svg)
**Português** | [English](./README.md)

![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue.svg)

## O que é o Axio

Axio é um logger estruturado para Go, voltado para observabilidade, auditoria e governança de dados. Padroniza campos, reduz risco de vazamento de dados sensíveis e permite correlação com tracing distribuído, sem acoplar sua aplicação ao motor interno de logging.

---

## Por que um Wrapper?

### O Problema

Dependência direta de bibliotecas de logging (Zap, Logrus, zerolog) acopla toda a aplicação a uma implementação específica. Mudanças no motor de logging exigem refatoração em dezenas de arquivos.

### A Solução

Axio funciona como uma camada de abstração com interface estável (`Logger`). O código de negócio depende apenas da interface Axio, não do motor interno.

### Vantagens da Abordagem

| Vantagem                | Descrição                                     |
| ----------------------- | --------------------------------------------- |
| Desacoplamento          | Código de negócio não conhece Zap             |
| Migração facilitada     | Trocar motor interno sem refatorar aplicações |
| Consistência            | Mesma API para todos os times/serviços        |
| Extensibilidade         | Hooks, métricas, tracing via composição       |
| Testabilidade           | Interface facilita mocks em testes            |
| Governança centralizada | PII, auditoria, formatos em um lugar só       |

### Arquitetura

```
┌─────────────────────────────────────────────────┝
│           Aplicação (código de negócio)         │
│                       ↓                         │
│             axio.Logger (interface)            │
│                       ↓                         │
│   ┌─────────────────────────────────────────┝   │
│   │              Axio Core                 │   │
│   │  ┌─────┝ ┌─────┝ ┌───────┝ ┌─────────┝ │   │
│   │  │ PII │ │Audit│ │Tracing│ │ Metrics │ │   │
│   │  └─────┘ └─────┘ └───────┘ └─────────┘ │   │
│   │                    ↓                    │   │
│   │          Motor de Logging               │   │
│   │          (Zap - substituível)           │   │
│   └─────────────────────────────────────────┘   │
│                       ↓                         │
│            Outputs (Console/File/Stdout)        │
└─────────────────────────────────────────────────┘
```

---

## Ýndice

- [Instalação](#instalação)
- [Exemplo Rápido](#exemplo-rápido)
- [Configuração](#configuração)
  - [Config Principal](#config-principal)
  - [OutputConfig](#outputconfig)
  - [AuditConfig](#auditconfig)
  - [MetricsConfig](#metricsconfig)
  - [Carregamento de Arquivo](#carregamento-de-arquivo)
- [Recursos](#recursos)
  - [Saídas (Outputs)](#saídas-outputs)
  - [Níveis de Log](#níveis-de-log)
  - [Anotações Estruturadas](#anotações-estruturadas)
  - [Hooks](#hooks)
  - [PII - Mascaramento de Dados Sensíveis](#pii---mascaramento-de-dados-sensíveis)
  - [Auditoria (Hash Chain)](#auditoria-hash-chain)
  - [Tracing Distribuído (OpenTelemetry)](#tracing-distribuído-opentelemetry)
  - [Métricas](#métricas)
- [Boas Práticas de Logging](#boas-práticas-de-logging)
- [Guia por Tipo de Serviço](#guia-por-tipo-de-serviço)
- [Exemplos e Anti-padrões](#exemplos-e-anti-padrões)
- [Troubleshooting](#troubleshooting)

---

## Instalação

```bash
go get github.com/pragmabits/axio
```

```go
import "github.com/pragmabits/axio"
```

---

## Exemplo Rápido

Handler HTTP completo com contexto, anotações e cleanup:

```go
package main

import (
    "context"
    "log"
    "net/http"
    "time"

    "github.com/pragmabits/axio"
)

var logger axio.Logger

func main() {
    var err error
    logger, err = axio.New(axio.Config{
        ServiceName:    "api-vendas",
        ServiceVersion: "1.0.0",
        Environment:    axio.Production,
        Level:          axio.LevelInfo,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer logger.Close()

    http.HandleFunc("/api/orders", handleOrder)
    http.ListenAndServe(":8080", nil)
}

func handleOrder(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    ctx := r.Context()

    // ... lógica de negócio ...

    logger.With(
        &axio.HTTP{
            Method:     r.Method,
            URL:        r.URL.Path,
            StatusCode: 201,
            LatencyMS:  time.Since(start).Milliseconds(),
            ClientIP:   r.RemoteAddr,
        },
        axio.Annotate("user_id", "usr_123"),
    ).Info(ctx, "pedido criado")

    w.WriteHeader(http.StatusCreated)
}
```

---

## Configuração

### Config Principal

| Campo               | Tipo             | Obrigatório | Padrão                     | Valores                                | Validação                                |
| ------------------- | ---------------- | ----------- | -------------------------- | -------------------------------------- | ---------------------------------------- |
| `ServiceName`       | `string`         | Não         | `""`                       | qualquer                               | -                                        |
| `ServiceVersion`    | `string`         | Não         | `""`                       | qualquer                               | -                                        |
| `Environment`       | `Environment`    | Não         | `development`              | `production`, `staging`, `development` | `ErrInvalidEnvironment` se inválido      |
| `InstanceID`        | `string`         | Não         | `""`                       | qualquer                               | -                                        |
| `Level`             | `Level`          | Não         | `info`                     | `debug`, `info`, `warn`, `error`       | `ErrInvalidLevel` se inválido            |
| `CallerSkip`        | `int`            | Não         | `0`                        | `>= 0`                                 | -                                        |
| `DisableSample`     | `bool`           | Não         | `false`                    | `true`, `false`                        | -                                        |
| `AgentMode`         | `bool`           | Não         | `false`                    | `true`, `false`                        | Se `true`, outputs devem ser stdout+json |
| `Outputs`           | `[]OutputConfig` | Não         | auto                       | ver OutputConfig                       | Validados individualmente                |
| `PIIEnabled`        | `bool`           | Não         | `false`                    | `true`, `false`                        | -                                        |
| `PIIPatterns`       | `[]PIIPattern`   | Não         | `[cpf, cnpj, credit_card]` | ver tabela PII                         | -                                        |
| `PIIFields`         | `[]string`       | Não         | `DefaultSensitiveFields`   | qualquer                               | -                                        |
| `PIICustomPatterns` | `[]CustomPII`    | Não         | `[]`                       | ver CustomPII                          | Regex deve ser válida                    |
| `TracerType`        | `string`         | Não         | `noop`                     | `otel`, `noop`                         | `ErrInvalidTracer` se inválido           |
| `Audit`             | `AuditConfig`    | Não         | desabilitado               | ver AuditConfig                        | -                                        |
| `Metrics`           | `MetricsConfig`  | Não         | desabilitado               | ver MetricsConfig                      | -                                        |

### OutputConfig

| Campo    | Tipo         | Obrigatório | Padrão | Valores                     | Validação                                    |
| -------- | ------------ | ----------- | ------ | --------------------------- | -------------------------------------------- |
| `Type`   | `OutputType` | Sim         | -      | `console`, `stdout`, `file` | `ErrInvalidOutputType` se inválido           |
| `Format` | `Format`     | Sim         | -      | `json`, `text`              | `ErrInvalidFormat` se inválido               |
| `Path`   | `string`     | Condicional | `""`   | caminho de arquivo          | `ErrFileOutputNoPath` se `Type=file` e vazio |

### AuditConfig

| Campo       | Tipo     | Obrigatório | Padrão  | Valores            | Validação                                       |
| ----------- | -------- | ----------- | ------- | ------------------ | ----------------------------------------------- |
| `Enabled`   | `bool`   | Não         | `false` | `true`, `false`    | -                                               |
| `StorePath` | `string` | Condicional | `""`    | caminho de arquivo | `ErrAuditWithoutPath` se `Enabled=true` e vazio |

### MetricsConfig

| Campo          | Tipo     | Obrigatório | Padrão  | Valores         | Validação |
| -------------- | -------- | ----------- | ------- | --------------- | --------- |
| `Enabled`      | `bool`   | Não         | `false` | `true`, `false` | -         |
| `MeterName`    | `string` | Não         | `axio`  | qualquer        | -         |
| `MeterVersion` | `string` | Não         | `1.0.0` | qualquer        | -         |

### Carregamento de Arquivo

Axio suporta configuração via YAML, JSON ou TOML:

```go
// Carregar de arquivo (detecta formato pela extensão)
config, err := axio.LoadConfig("/etc/axio/config.yaml")

// Carregar de io.Reader (especifica formato)
config, err := axio.LoadConfigFrom(reader, "yaml")

// Versão que entra em pânico (útil em main)
config := axio.MustLoadConfig("/etc/axio/config.yaml")
```

**Exemplo YAML completo:**

```yaml
serviceName: api-vendas
serviceVersion: 2.1.0
environment: production
instanceId: pod-abc123
level: info
callerSkip: 0
agentMode: false

outputs:
  - type: stdout
    format: json
  - type: file
    format: json
    path: /var/log/app.log

piiEnabled: true
piiPatterns:
  - cpf
  - cnpj
  - email
  - credit_card
piiFields:
  - password
  - token
  - secret

piiCustomPatterns:
  - name: matricula
    pattern: "MAT-\\d{6}"
    mask: "MAT-******"

audit:
  enabled: true
  storePath: /var/lib/axio/chain.json

tracer: otel

metrics:
  enabled: true
  meterName: axio
  meterVersion: 1.0.0
```

---

## Recursos

### Saídas (Outputs)

#### Tipos de Output

| Tipo      | Destino | Uso típico                       |
| --------- | ------- | -------------------------------- |
| `console` | stderr  | Desenvolvimento local            |
| `stdout`  | stdout  | Containers com agentes de coleta |
| `file`    | arquivo | Ambientes sem agentes, auditoria |

#### Formatos

| Formato | Descrição        | Uso                             |
| ------- | ---------------- | ------------------------------- |
| `json`  | JSON estruturado | Produção, sistemas de agregação |
| `text`  | Texto colorido   | Desenvolvimento local           |

#### Comportamento por Ambiente

| Ambiente      | Output Padrão | Formato | Stack Trace |
| ------------- | ------------- | ------- | ----------- |
| `development` | Console       | Text    | Não         |
| `staging`     | Stdout        | JSON    | Em erros    |
| `production`  | Stdout        | JSON    | Em erros    |

#### Configuração via Options

```go
// Múltiplos outputs
logger, _ := axio.New(config,
    axio.WithOutputs(
        axio.Console(axio.FormatText),
        axio.Stdout(axio.FormatJSON),
        axio.MustFile("/var/log/app.log", axio.FormatJSON),
    ),
)

// Modo agente (stdout + JSON, otimizado para Promtail, Fluent Bit, etc.)
logger, _ := axio.New(config, axio.WithAgentMode())
```

---

### Níveis de Log

| Nível | Constante    | Semântica              | Quando usar                      |
| ----- | ------------ | ---------------------- | -------------------------------- |
| Debug | `LevelDebug` | Detalhes técnicos      | Desenvolvimento, troubleshooting |
| Info  | `LevelInfo`  | Eventos normais        | Início/fim de operações, marcos  |
| Warn  | `LevelWarn`  | Anomalias não-críticas | Timeouts, fallbacks, degradação  |
| Error | `LevelError` | Falhas reais           | Operação falhou, requer atenção  |

**Métodos:**

```go
logger.Debug(ctx, "detalhes de depuração")
logger.Info(ctx, "processados %d itens", count)
logger.Warn(ctx, err, "timeout ao consultar fornecedor")
logger.Error(ctx, err, "falha ao persistir pedido")
```

---

### Anotações Estruturadas

#### Annotate

Adiciona campos chave-valor ao log:

```go
logger.With(
    axio.Annotate("user_id", "usr_123"),
    axio.Annotate("order_id", "ord_456"),
    axio.Annotate("amount_cents", 15000),
).Info(ctx, "pedido criado")
```

#### HTTP

Struct para metadados de requisições HTTP:

```go
logger.With(&axio.HTTP{
    Method:     "POST",
    URL:        "/api/v1/orders",
    StatusCode: 201,
    LatencyMS:  45,
    UserAgent:  r.UserAgent(),
    ClientIP:   r.RemoteAddr,
}).Info(ctx, "requisição processada")
```

| Campo        | Tipo     | Descrição                     |
| ------------ | -------- | ----------------------------- |
| `Method`     | `string` | Método HTTP (GET, POST, etc.) |
| `URL`        | `string` | Caminho da requisição         |
| `StatusCode` | `int`    | Código de resposta            |
| `LatencyMS`  | `int64`  | Latência em milissegundos     |
| `UserAgent`  | `string` | User-Agent do cliente         |
| `ClientIP`   | `string` | IP do cliente                 |

#### Marshaler (customizado)

Implemente `Marshaler` para tipos complexos:

```go
type Order struct {
    ID     string
    Items  []Item
    secret string // não será logado
}

func (o Order) MarshalLog(a axio.Annotator) error {
    a.Add("order_id", o.ID)
    a.Add("item_count", len(o.Items))
    return nil
}

// Uso
logger.With(axio.Annotate("order", order)).Info(ctx, "pedido processado")
```

#### Named (sub-loggers)

Cria loggers com namespace:

```go
httpLogger := logger.Named("http")
dbLogger := logger.Named("db")
cacheLogger := logger.Named("cache")

httpLogger.Info(ctx, "requisição recebida")  // logger: "http"
dbLogger.Info(ctx, "query executada")        // logger: "db"
```

---

### Hooks

Hooks processam entradas de log antes da escrita. Executados em ordem fixa:

1. **PIIHook** - mascara dados sensíveis
2. **AuditHook** - calcula hash chain
3. **Hooks customizados** - na ordem passada para `WithHooks`

#### Interface Hook

```go
type Hook interface {
    Name() string
    Process(ctx context.Context, entry *Entry) error
}
```

#### Hook Customizado

```go
type TenantHook struct {
    tenantID string
}

func (h TenantHook) Name() string { return "tenant" }

func (h TenantHook) Process(ctx context.Context, entry *axio.Entry) error {
    entry.Annotations = append(entry.Annotations,
        axio.Annotate("tenant_id", h.tenantID))
    return nil
}

// Uso
logger, _ := axio.New(config, axio.WithHooks(TenantHook{tenantID: "acme"}))
```

---

### PII - Mascaramento de Dados Sensíveis

#### O que é PII?

**PII** (Personally Identifiable Information) ou **Informação Pessoal Identificável** é qualquer dado que pode identificar uma pessoa, direta ou indiretamente. Exemplos: CPF, CNPJ, e-mail, telefone, endereço IP, números de cartão.

Em ambientes com logs centralizados, PII exposta representa risco de:
- Vazamento de dados
- Não-conformidade com LGPD/GDPR
- Exposição em incidentes de segurança

**Referências:**
- [LGPD - Lei 13.709/2018](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm)
- [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)

#### Padrões Builtin

| Padrão          | Constante           | Formatos detectados             | Máscara               |
| --------------- | ------------------- | ------------------------------- | --------------------- |
| CPF             | `PatternCPF`        | `123.456.789-01`, `12345678901` | `***.***.***-**`      |
| CNPJ            | `PatternCNPJ`       | `12.345.678/0001-90`            | `**.***.***/**01-**`  |
| Cartão          | `PatternCreditCard` | `1234-5678-9012-3456`           | `****-****-****-****` |
| Email           | `PatternEmail`      | `user@domain.com`               | `***@***.***`         |
| Telefone        | `PatternPhone`      | `(11) 99999-9999`               | `(**) *****-****`     |
| Telefone s/ DDD | `PatternPhoneNoDDD` | `99999-9999`                    | `*****-****`          |

#### Campos Sensíveis Automáticos

Campos cujos nomes contenham estes termos são automaticamente redigidos para `[REDACTED]`:

`password`, `senha`, `token`, `api_key`, `apikey`, `secret`, `credential`, `authorization`, `bearer`, `private_key`, `privatekey`, `access_key`, `secret_key`, `client_secret`, `clientsecret`

#### Configuração

```go
// Via Options (recomendado)
logger, _ := axio.New(config,
    axio.WithPII(
        []axio.PIIPattern{axio.PatternCPF, axio.PatternEmail},
        axio.DefaultSensitiveFields,
    ),
)

// Via Hook direto
hook := axio.MustPIIHook(axio.DefaultPIIConfig())
logger, _ := axio.New(config, axio.WithHooks(hook))

// Via Config (arquivo YAML)
// piiEnabled: true
// piiPatterns: [cpf, cnpj, email]
```

#### Padrões Customizados

```go
config := axio.PIIConfig{
    Patterns: []axio.PIIPattern{axio.PatternCPF},
    CustomPatterns: []axio.CustomPII{
        {
            Name:    "matricula",
            Pattern: `MAT-\d{6}`,
            Mask:    "MAT-******",
        },
    },
    Fields: axio.DefaultSensitiveFields,
}
```

---

### Auditoria (Hash Chain)

#### O que é Hash Chain?

Uma **hash chain** (cadeia de hashes) é uma estrutura onde cada registro contém o hash criptográfico do registro anterior. Qualquer modificação em um registro quebra toda a cadeia subsequente, permitindo detectar adulteração.

Útil para:
- Conformidade regulatória (LGPD, SOX, PCI-DSS)
- Logs de auditoria à prova de adulteração
- Evidência de integridade em investigações

**Importante:** Hash chain detecta alteração, não previne. A imutabilidade depende do backend de armazenamento.

#### Campos Adicionados

| Campo       | Descrição                 |
| ----------- | ------------------------- |
| `hash`      | Hash SHA256 desta entrada |
| `prev_hash` | Hash da entrada anterior  |

#### Configuração

```go
// Via Options
logger, _ := axio.New(config,
    axio.WithAudit("/var/lib/axio/chain.json"),
)

// Via Hook direto
store := axio.NewFileStore("/var/lib/axio/chain.json")
hook, _ := axio.NewAuditHook(store)
logger, _ := axio.New(config, axio.WithHooks(hook))
```

#### ChainStore Customizado

Implemente `ChainStore` para backends customizados (Redis, PostgreSQL, etc.):

```go
type ChainStore interface {
    Save(sequence uint64, lastHash string) error
    Load() (sequence uint64, lastHash string, err error)
}
```

---

### Tracing Distribuído (OpenTelemetry)

#### O que é Tracing Distribuído?

**Tracing distribuído** permite rastrear uma requisição através de múltiplos serviços. Cada operação recebe um **span** identificado por:

- **trace_id**: identificador único da requisição completa
- **span_id**: identificador único desta operação específica

Com esses IDs nos logs, é possível correlacionar logs e traces em ferramentas como Jaeger, Tempo ou Zipkin.

#### Por que OpenTelemetry?

O Axio usa OpenTelemetry (OTel) como padrão para tracing pelos seguintes motivos:

| Fator              | OpenTelemetry                                                      |
| ------------------ | ------------------------------------------------------------------ |
| **Padronização**   | Projeto oficial CNCF, padrão da indústria                          |
| **Vendor-neutral** | Funciona com qualquer backend (Jaeger, Zipkin, Datadog, AWS X-Ray) |
| **Unificação**     | Traces, métricas e logs em uma única API                           |
| **Adoção**         | AWS, GCP, Azure, Datadog, Grafana, todos suportam                  |
| **Comunidade**     | Desenvolvimento ativo, ampla documentação                          |
| **Futuro**         | Sucessor oficial de OpenTracing e OpenCensus                       |

**Alternativas consideradas:**
- **Jaeger client**: específico para Jaeger, descontinuado em favor de OTel
- **Zipkin**: menos flexível, sem unificação de sinais
- **Proprietários**: lock-in com vendor específico

**Referências:**
- [OpenTelemetry](https://opentelemetry.io/)
- [OTel Go](https://opentelemetry.io/docs/languages/go/)
- [CNCF - OpenTelemetry](https://www.cncf.io/projects/opentelemetry/)

#### Configuração

```go
// Via Options (recomendado)
logger, _ := axio.New(config, axio.WithTracer(axio.Otel()))

// Via Config (arquivo YAML)
// tracer: otel

// Desabilitar (padrão)
logger, _ := axio.New(config, axio.WithTracer(axio.NoopTracing()))
```

#### Uso com Span Ativo

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // contém span do middleware OTel

    logger.Info(ctx, "requisição recebida")
    // Log incluirá: {"trace_id": "abc123...", "span_id": "def456..."}
}
```

---

### Métricas

#### O que são Métricas de Observabilidade?

**Métricas** são valores numéricos que representam o estado do sistema ao longo do tempo. Tipos comuns:
- **Contadores**: valores que só aumentam (ex: total de logs)
- **Histogramas**: distribuição de valores (ex: duração de hooks)

Axio emite métricas sobre o próprio processo de logging, permitindo monitorar volume, erros e performance.

**Referências:**
- [OTel Metrics](https://opentelemetry.io/docs/concepts/signals/metrics/)
- [Prometheus](https://prometheus.io/docs/concepts/metric_types/)

#### Métricas Emitidas

| Métrica         | Tipo      | Labels               | Descrição                      |
| --------------- | --------- | -------------------- | ------------------------------ |
| `logs.total`    | Counter   | `level`              | Total de logs emitidos         |
| `pii.masked`    | Counter   | `pattern`            | Ocorrências de PII mascaradas  |
| `audit.records` | Counter   | -                    | Registros de auditoria criados |
| `output.errors` | Counter   | `output.type`        | Erros de escrita em outputs    |
| `hook.duration` | Histogram | `hook.name`, `error` | Duração de execução de hooks   |

#### Configuração

```go
// Via Options com MeterProvider
provider := otel.GetMeterProvider()
logger, _ := axio.New(config, axio.WithMetrics(provider))

// Via Config (usa provider global com warning)
// metrics:
//   enabled: true
//   meterName: axio
//   meterVersion: 1.0.0
```

#### Interface Metrics (customizada)

```go
type Metrics interface {
    LogsTotal(ctx context.Context, level Level)
    PIIMasked(ctx context.Context, pattern PIIPattern)
    AuditRecords(ctx context.Context)
    OutputErrors(ctx context.Context, output OutputType)
    HookDuration(ctx context.Context, hookName string, duration time.Duration)
    HookDurationWithError(ctx context.Context, hookName string, duration time.Duration, hasError bool)
}
```

---

## Boas Práticas de Logging

### 1. Estrutura antes de texto

- Use campos estruturados para dados; mensagem é resumo humano
- Prefira chaves estáveis: `user_id`, `order_id`, `tenant_id`
- Evite chaves dinâmicas: `field_123`, `user_email_john@...`

### 2. Níveis com semântica clara

| Nível | Use quando                       |
| ----- | -------------------------------- |
| Debug | Detalhes técnicos, temporários   |
| Info  | Eventos normais, marcos de fluxo |
| Warn  | Anomalias que não interrompem    |
| Error | Falha real da operação           |

**Regra:** Logue erro uma vez, no limite do sistema (handler, job, consumer).

### 3. Contexto e correlação

Sempre passe `context.Context` e adicione identificadores:

- `request_id` / `correlation_id`
- `user_id`, `tenant_id`
- `trace_id`, `span_id` (via tracing)

### 4. PII e dados sensíveis

- Use `PIIHook` como defesa padrão
- Nunca logue: senha, token, segredo, chave privada
- Se precisar do payload, logue hash ou ID, não o conteúdo

### 5. Performance e custo

- Evite logs em loops quentes; prefira agregação
- Não construa strings/mapas grandes desnecessariamente
- Em produção: JSON + coleta por agente

### 6. Cardinalidade controlada

Campos com valores ilimitados (email, payloads) explodem índices. Mantenha:

- IDs estáveis (user, order, tenant)
- Status codes, métodos, endpoints
- Latência em milissegundos

### 7. Auditoria e integridade

Para operações críticas, use `AuditHook` e combine com armazenamento confiável.

### 8. Campos HTTP padrão

```go
logger.With(
    &axio.HTTP{
        Method:     r.Method,
        URL:        r.URL.Path,
        StatusCode: statusCode,
        LatencyMS:  latencyMS,
        ClientIP:   r.RemoteAddr,
    },
    axio.Annotate("request_id", requestID),
    axio.Annotate("user_id", userID),
).Info(ctx, "requisição concluída")
```

### 9. Checklist de review

- [ ] Mensagem resume o evento?
- [ ] Campos são consistentes e estáveis?
- [ ] Nível está correto?
- [ ] Há PII exposta?
- [ ] Erro foi logado uma única vez?

---

## Guia por Tipo de Serviço

### APIs HTTP/gRPC

**Objetivo:** Medir latência, sucesso/erro, rastrear requisições.

| Evento               | Nível      | Campos sugeridos                              |
| -------------------- | ---------- | --------------------------------------------- |
| Requisição concluída | Info       | `http.*`, `request_id`, `user_id`, `trace_id` |
| Erro de domínio      | Warn/Error | `+operation`, `+entity`, `+error`             |

```go
logger.With(&axio.HTTP{...}, axio.Annotate("request_id", id)).Info(ctx, "requisição finalizada")
```

### Workers e Jobs

**Objetivo:** Saber quando iniciou, terminou, quanto processou.

| Evento        | Nível | Campos sugeridos                                             |
| ------------- | ----- | ------------------------------------------------------------ |
| Job iniciado  | Info  | `job_name`, `job_id`                                         |
| Job concluído | Info  | `+items_total`, `+items_ok`, `+items_failed`, `+duration_ms` |
| Erro em item  | Warn  | `+item_id`, `+error` (amostrado)                             |

```go
logger.With(
    axio.Annotate("job_name", "reconcile_payments"),
    axio.Annotate("items_ok", okCount),
    axio.Annotate("items_failed", failedCount),
).Info(ctx, "job concluído")
```

### Consumidores de Filas

**Objetivo:** Rastrear consumo, retries, falhas por mensagem.

| Evento              | Nível      | Campos sugeridos                    |
| ------------------- | ---------- | ----------------------------------- |
| Mensagem processada | Info/Debug | `queue`, `message_id`, `latency_ms` |
| Falha em mensagem   | Warn/Error | `+retry_count`, `+error`            |

### Integrações Externas

**Objetivo:** Visibilidade de latência e falhas em terceiros.

| Evento          | Nível      | Campos sugeridos                                     |
| --------------- | ---------- | ---------------------------------------------------- |
| Chamada externa | Info/Debug | `provider`, `operation`, `status_code`, `latency_ms` |
| Timeout/erro    | Warn       | `+attempt`, `+timeout_ms`                            |

### CLIs e Scripts

**Objetivo:** Auditar execução e resultado.

| Evento | Nível | Campos sugeridos                              |
| ------ | ----- | --------------------------------------------- |
| Início | Info  | `command`, `args_redacted`                    |
| Fim    | Info  | `+exit_code`, `+duration_ms`, `+output_count` |

---

## Exemplos e Anti-padrões

### Anti-padrão: Concatenação para dados estruturados

**Errado:**
```go
logger.Info(ctx, "usuario=%s status=%d", userID, statusCode)
```

**Correto:**
```go
logger.With(
    axio.Annotate("user_id", userID),
    axio.Annotate("status_code", statusCode),
).Info(ctx, "requisição concluída")
```

### Anti-padrão: Payload com PII

**Errado:**
```go
logger.Info(ctx, "payload=%+v", payload)
```

**Correto:**
```go
logger.With(
    axio.Annotate("payload_id", payload.ID),
    axio.Annotate("payload_size", len(payload.Data)),
).Info(ctx, "payload recebido")
```

### Anti-padrão: Log duplicado em camadas

**Errado:**
```go
// repository
if err != nil {
    logger.Error(ctx, err, "falha ao inserir")
    return err
}
```

**Correto:**
```go
// repository
if err != nil {
    return fmt.Errorf("insert pedido: %w", err)
}

// handler (limite do sistema)
if err != nil {
    logger.Error(ctx, err, "falha ao criar pedido")
}
```

### Anti-padrão: Log em loop quente

**Errado:**
```go
for _, item := range items {
    logger.Debug(ctx, "processando item %s", item.ID)
}
```

**Correto:**
```go
logger.With(
    axio.Annotate("items_total", len(items)),
    axio.Annotate("items_ok", okCount),
    axio.Annotate("items_failed", failedCount),
).Info(ctx, "lote processado")
```

### Anti-padrão: Cardinalidade explosiva

**Errado:**
```go
logger.With(axio.Annotate("email", user.Email)).Info(ctx, "login")
```

**Correto:**
```go
logger.With(axio.Annotate("user_id", user.ID)).Info(ctx, "login")
```

### Anti-padrão: Mensagem vaga

**Errado:**
```go
logger.Error(ctx, err, "erro")
```

**Correto:**
```go
logger.With(
    axio.Annotate("order_id", order.ID),
).Error(ctx, err, "falha ao confirmar pagamento")
```

---

## Troubleshooting

### Tabela de Erros

| Erro                     | Causa                                | Solução                                      |
| ------------------------ | ------------------------------------ | -------------------------------------------- |
| `ErrInvalidEnvironment`  | Valor de Environment inválido        | Use `production`, `staging` ou `development` |
| `ErrInvalidLevel`        | Valor de Level inválido              | Use `debug`, `info`, `warn` ou `error`       |
| `ErrInvalidFormat`       | Valor de Format inválido             | Use `json` ou `text`                         |
| `ErrInvalidOutputType`   | Valor de OutputType inválido         | Use `console`, `stdout` ou `file`            |
| `ErrIncompatibleOutputs` | AgentMode com output não-stdout/json | Em AgentMode, use apenas stdout + json       |
| `ErrFileOutputNoPath`    | Output tipo file sem path            | Especifique `path` no OutputConfig           |
| `ErrAuditWithoutPath`    | Audit habilitado sem storePath       | Especifique `storePath` no AuditConfig       |
| `ErrInvalidTracer`       | Valor de TracerType inválido         | Use `otel` ou `noop`                         |
| `ErrLoadConfig`          | Falha ao ler arquivo de config       | Verifique caminho e permissões               |
| `ErrUnknownFormat`       | Extensão de arquivo desconhecida     | Use `.yaml`, `.yml`, `.json` ou `.toml`      |
| `ErrUnmarshalConfig`     | Falha ao fazer parse do config       | Verifique sintaxe do arquivo                 |
| `ErrApplyOption`         | Falha ao aplicar Option              | Verifique parâmetros da Option               |
| `ErrValidateConfig`      | Configuração inválida após Options   | Verifique combinação de valores              |
| `ErrBuildOutputs`        | Falha ao criar outputs               | Verifique caminhos de arquivo                |
| `ErrBuildHooks`          | Falha ao criar hooks                 | Verifique regex de PIICustomPatterns         |
| `ErrBuildMetrics`        | Falha ao construir métricas          | Verifique configuração do MeterProvider      |
| `ErrBuildEngine`         | Falha ao construir engine de logging | Verifique combinação de outputs e config     |
| `ErrOpenFile`            | Falha ao abrir arquivo de log        | Verifique caminho e permissões               |
| `ErrLoadChainState`      | Falha ao carregar estado da chain    | Verifique arquivo de chain                   |
| `ErrSaveChainState`      | Falha ao salvar estado da chain      | Verifique permissões de escrita              |
| `ErrMarshalChainState`   | Falha ao serializar estado da chain  | Erro interno de serialização                 |
| `ErrUnmarshalChainState` | Falha ao desserializar estado da chain | Arquivo de chain corrompido ou formato inválido |
| `ErrHashMismatch`        | Hash calculado não corresponde       | Cadeia de auditoria corrompida               |
| `ErrChainBroken`         | Integridade da cadeia comprometida   | Registros foram adulterados                  |
| `ErrSerializeEntry`      | Falha ao serializar entrada de audit | Entrada contém dados não serializáveis       |
| `ErrCreateAuditHook`     | Falha ao criar hook de auditoria     | Verifique configuração do chain store        |
| `ErrNilMetricsProvider`  | Provider de métricas é nil           | Passe um MeterProvider válido                |
| `ErrCreateMetric`        | Falha ao criar instrumento OTel      | Verifique configuração do provider           |
