# Axio

![Português](https://img.shields.io/badge/lang-pt--BR-green.svg)
**Português** | [English](./README.md)

![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-0BSD-blue.svg)

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

## Índice

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
- [Linha de Comando: axio render e axio verify](#linha-de-comando-axio-render-e-axio-verify)
- [Boas Práticas de Logging](#boas-práticas-de-logging)
- [Guia por Tipo de Serviço](#guia-por-tipo-de-serviço)
- [Exemplos e Anti-padrões](#exemplos-e-anti-padrões)
- [Troubleshooting](#troubleshooting)

---

## Instalação

Requer Go 1.27 ou mais recente.

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
        Environment:    axio.EnvironmentProduction,
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

    logger.Info(ctx, "pedido criado",
        axio.Field("http", axio.HTTP{
            Method:     r.Method,
            URL:        r.URL.Path,
            StatusCode: 201,
            LatencyMS:  time.Since(start).Milliseconds(),
            ClientIP:   r.RemoteAddr,
        }),
        axio.Field("user_id", "usr_123"),
    )

    w.WriteHeader(http.StatusCreated)
}
```

---

## Configuração

### Config Principal

| Campo                 | Tipo             | Obrigatório | Padrão                     | Valores                                | Validação                                |
| --------------------- | ---------------- | ----------- | -------------------------- | -------------------------------------- | ---------------------------------------- |
| `ServiceName`         | `string`         | Não         | `""`                       | qualquer                               | -                                        |
| `ServiceVersion`      | `string`         | Não         | `""`                       | qualquer                               | -                                        |
| `Environment`         | `Environment`    | Não         | `development`              | `production`, `staging`, `development` | `ErrInvalidEnvironment` se inválido      |
| `InstanceID`          | `string`         | Não         | `""`                       | qualquer                               | -                                        |
| `Level`               | `Level`          | Não         | `info`                     | `debug`, `info`, `warn`, `error`       | `ErrInvalidLevel` se inválido            |
| `CallerSkip`          | `int`            | Não         | `0`                        | `>= 0`                                 | -                                        |
| `AgentMode`           | `bool`           | Não         | `false`                    | `true`, `false`                        | Se `true`, outputs devem ser stdout+json |
| `Outputs`             | `[]OutputConfig` | Não         | auto                       | ver OutputConfig                       | Validados individualmente                |
| `PIIEnabled`          | `bool`           | Não         | `false`                    | `true`, `false`                        | -                                        |
| `PIIPatterns`         | `[]PIIPattern`   | Não         | `[cpf, cnpj, credit_card]` | ver tabela PII                         | -                                        |
| `PIIFields`           | `[]string`       | Não         | `DefaultSensitiveFields()` | qualquer                               | -                                        |
| `PIIMaxDepth`         | `int`            | Não         | `0` (= `32`)               | `>= 0`                                 | `ErrInvalidPIIMaxDepth` se negativo      |
| `PIIOmitErrorVerbose` | `bool`           | Não         | `false`                    | `true`, `false`                        | -                                        |
| `PIICustomPatterns`   | `[]CustomPII`    | Não         | `[]`                       | ver CustomPII                          | Regex deve ser válida                    |
| `TracerType`          | `string`         | Não         | `noop`                     | `otel`, `noop`                         | `ErrInvalidTracer` se inválido           |
| `Audit`               | `AuditConfig`    | Não         | desabilitado               | ver AuditConfig                        | -                                        |
| `Metrics`             | `MetricsConfig`  | Não         | desabilitado               | ver MetricsConfig                      | -                                        |

### OutputConfig

| Campo      | Tipo             | Obrigatório | Padrão     | Valores                     | Validação                                    |
| ---------- | ---------------- | ----------- | ---------- | --------------------------- | -------------------------------------------- |
| `Type`     | `OutputType`     | Sim         | -          | `console`, `stdout`, `file` | `ErrInvalidOutputType` se inválido           |
| `Format`   | `Format`         | Sim         | -          | `json`, `text`              | `ErrInvalidFormat` se inválido               |
| `Path`     | `string`         | Condicional | `""`       | caminho de arquivo          | `ErrFileOutputNoPath` se `Type=file` e vazio |
| `Rotation` | `RotationConfig` | Não         | desativada | ver RotationConfig          | Aplicável apenas quando `Type=file`          |

### RotationConfig

| Campo        | Tipo       | Obrigatório | Padrão  | Valores                            | Validação |
| ------------ | ---------- | ----------- | ------- | ---------------------------------- | --------- |
| `MaxSize`    | `int`      | Não         | `0`     | megabytes (0 = sem limite)         | -         |
| `MaxAge`     | `int`      | Não         | `0`     | dias (0 = sem limite)              | -         |
| `MaxBackups` | `int`      | Não         | `0`     | quantidade (0 = manter todos)      | -         |
| `Compress`   | `bool`     | Não         | `false` | `true`, `false`                    | -         |
| `LocalTime`  | `bool`     | Não         | `false` | `true`, `false`                    | -         |
| `Interval`   | `Duration` | Não         | `0`     | ex.: `24h`, `1h30m`, `500ms`       | -         |

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
    rotation:
      maxSize: 100
      maxAge: 30
      maxBackups: 10
      compress: true
      interval: 24h

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
piiMaxDepth: 8
piiOmitErrorVerbose: false

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

As options vencem o arquivo de config: o primeiro `WithOutputs` substitui os `outputs` do arquivo, que então não são abertos nem validados, e as chamadas seguintes acrescentam.

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
logger.Info(ctx, "itens processados", axio.Field("count", count))
logger.Warn(ctx, err, "timeout ao consultar fornecedor")
logger.Error(ctx, err, "falha ao persistir pedido")
```

A mensagem é escrita como foi passada, nunca formatada. Os dados vão em anotações passadas depois dela, que a entrada carrega depois das do logger (`With`) e que nenhuma outra chamada vê.

---

### Anotações Estruturadas

#### Field

Adiciona campos chave-valor ao log:

```go
logger.Info(ctx, "pedido criado",
    axio.Field("user_id", "usr_123"),
    axio.Field("order_id", "ord_456"),
    axio.Field("amount_cents", 15000),
)
```

Uma anotação com o nome de uma chave que o próprio axio escreve — `timestamp`, `level`, `message`, `logger`, `caller`, `stacktrace`, `service`, `deployment`, `trace_id`, `span_id`, `error` (com `errorVerbose` e `errorCauses`), `event`, `duration_ms`, `previous_hash`, `hash` — sai com um sublinhado na frente, como `_message`, para que uma linha nunca repita uma chave.

Um struct, um slice ou um mapa sai como a sua codificação JSON, em `encoding/json/v2`: slices e mapas nil como `null`, chaves de mapa em ordem, um `time.Duration` em nanossegundos e um array de bytes em base64. `omitempty` omite um campo cujo valor codifica como vazio — `""`, `null`, `[]`, `{}` — e `omitzero` omite `false`, `0` e todo outro valor zero. Uma opção de tag que a codificação não aceita, como `,string` num slice, faz o valor falhar: a linha leva `<chave>Error` no lugar dele.

#### With

Devolve um logger que anexa as suas anotações a toda entrada que escreve, antes das que cada chamada passa. Serve para campos que valem por várias linhas, como o ID de uma requisição; os campos de uma linha só vão na própria chamada:

```go
requestLogger := logger.With(axio.Field("request_id", requestID))

requestLogger.Info(ctx, "pedido criado", axio.Field("user_id", userID))
requestLogger.Error(ctx, err, "falha no pagamento")
```

#### HTTP

Struct para metadados de requisições HTTP:

```go
logger.Info(ctx, "requisição processada", axio.Field("http", axio.HTTP{
    Method:     "POST",
    URL:        "/api/v1/orders",
    StatusCode: 201,
    LatencyMS:  45,
    UserAgent:  r.UserAgent(),
    ClientIP:   r.RemoteAddr,
}))
```

| Campo        | Tipo     | Descrição                     |
| ------------ | -------- | ----------------------------- |
| `Method`     | `string` | Método HTTP (GET, POST, etc.) |
| `URL`        | `string` | Caminho da requisição         |
| `StatusCode` | `int`    | Código de resposta            |
| `LatencyMS`  | `int64`  | Latência em milissegundos     |
| `UserAgent`  | `string` | User-Agent do cliente         |
| `ClientIP`   | `string` | IP do cliente                 |

#### Annotable (tipos customizados)

Implemente `Annotable` para tipos que produzem múltiplos campos:

```go
type Order struct {
    ID     string
    Items  []Item
    secret string // não será logado
}

func (o Order) Append(target []axio.Annotation) []axio.Annotation {
    return append(target,
        axio.Field("order_id", o.ID),
        axio.Field("item_count", len(o.Items)),
    )
}

// Uso — os campos são expandidos individualmente na saída do log
logger.Info(ctx, "pedido processado", axio.Field("order", order))
```

Os campos são expandidos antes de qualquer hook rodar, então o mascaramento de PII e os hooks customizados veem cada um.

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
2. **Hooks customizados** - na ordem passada para `WithHooks`

Auditoria não é um hook: o hash é calculado quando a entrada é escrita, depois de todos os hooks, e por isso cobre o que eles mudaram.

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
        axio.Field("tenant_id", h.tenantID))
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
| CNPJ            | `PatternCNPJ`       | `12.345.678/0001-90`            | `**.***.***/****-**`  |
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
        axio.DefaultSensitiveFields(),
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
    Fields: axio.DefaultSensitiveFields(),
}
```

#### Cobertura

O mascaramento de PII cobre todo valor que uma linha carrega:

- **A mensagem.**
- **O erro** passado a `Warn`, `Error` ou `Event.SetError`, pela mensagem dele e, num erro que se formata sozinho, pela forma verbosa (`errorVerbose`). Um erro mascarado continua desembrulhando no original, então `errors.Is` segue funcionando nos hooks seguintes.
- **Nomes de anotação que casam com `PIIConfig.Fields`** — o valor inteiro vira `[REDACTED]`, qualquer que seja o tipo.
- **Strings, erros e valores `fmt.Stringer`**, pelo texto.
- **Bytes (`[]byte`)**, que saem em base64, pelo texto que carregam: texto mascarado continua bytes, e bytes que não são texto UTF-8 não podem ser inspecionados e viram `[REDACTED]`, sejam uma anotação própria, sejam campo ou elemento de um valor estruturado.
- **Texto em base64.** Toda string com forma de base64, no alfabeto padrão ou no de URL, com ou sem padding — a mensagem, o erro, uma anotação, um valor dentro de mapa ou struct — também é decodificada, e mascarada quando o texto decodificado tem PII. É assim que chegam um `[]byte` de texto, um array de bytes ou um tipo nomeado de slice de bytes dentro de um valor estruturado, que a codificação JSON carrega em base64. Uma string que decodifica para dados binários vira `[REDACTED]` quando algum padrão casa dentro deles, e passa como está caso contrário: nada distingue o base64 de outros dados binários de outra string com a mesma forma.
- **JWTs e JWEs.** Um token em qualquer ponto de um texto — mensagem, query de URL, anotação — vira `[REDACTED]` inteiro: é uma credencial, e as claims podem carregar o que nenhum padrão reconhece. Uma string é tomada por token quando tem a forma de um e o header decodifica para um objeto JSON que nomeia um algoritmo (`alg`), como todo header JOSE.
- **Falhas de codificação.** Um valor cuja codificação falha — um `MarshalJSON`, `MarshalLogObject` ou `MarshalLogArray` que devolve erro — tem esse erro mascarado onde é escrito, em `<chave>Error`, e a parte que chegou a escrever mascarada como qualquer valor.
- **Valores estruturados** — mapas, slices, structs, ponteiros, `http.Header` — percorridos pela codificação JSON que o log escreve para eles: em cada nível, as chaves são checadas contra `Fields` e as strings contra os padrões.
- **Valores `Annotable`** como o `HTTP`, expandidos nos seus campos antes de qualquer hook rodar.

Um valor estruturado que precisou de máscara é escrito como a árvore JSON mascarada, com as chaves dos objetos em ordem alfabética; um que não tinha nada a mascarar mantém a forma original. Um contêiner aninhado além do limite de profundidade (padrão `32`) vira `[REDACTED]` inteiro, nunca sai sem máscara. O limite se ajusta com `axio.WithPIIMaxDepth(n)`, com `piiMaxDepth` no arquivo de config, ou com `PIIConfig.MaxDepth` ao montar um `PIIMasker` ou `PIIHook` à mão.

A forma verbosa de um erro que se formata sozinho — o `errorVerbose` ao lado da mensagem, uma pilha para muitos erros — é mascarada e mantida. Mascarar significa passar todos os padrões por ela: com os padrões default, cerca de 250 µs para uma pilha de uns 2 KB. Para omiti-la, sem nunca lê-la, use `axio.WithPIIOmitErrorVerbose()`, `piiOmitErrorVerbose: true` no arquivo de config, ou `PIIConfig.OmitErrorVerbose`; o erro sai então só pela mensagem mascarada.

```go
type Usuario struct {
    Email string `json:"email"`
    Senha string `json:"senha"`
}

logger.Info(ctx, "...", axio.Field("usuario", Usuario{Email: "a@b.com", Senha: "x"}))
//   -> "usuario": {"email": "***@***.***", "senha": "[REDACTED]"}   (com PatternEmail)
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

#### Como o axio calcula o hash de uma linha

Cada linha JSON auditada termina com dois campos, sempre por último:

| Campo           | Descrição                                                          |
| --------------- | ------------------------------------------------------------------ |
| `previous_hash` | Hash da linha anterior; vazio na primeira linha da cadeia          |
| `hash`          | SHA-256 de `previous_hash` seguido de todos os bytes antes do fim  |

O hash cobre exatamente o que foi escrito: mensagem, erro, stacktrace, metadados do serviço, anotações e o que os hooks customizados mudaram. Codificar, calcular o hash e escrever acontecem sob um único lock, então a ordem da cadeia é a ordem do arquivo, mesmo com várias goroutines escrevendo ao mesmo tempo.

```json
{"level":"info","timestamp":"2026-09-23T15:16:24.230105632Z","logger":"orders","caller":"app/main.go:26","message":"order created","order_id":"ord_8812","previous_hash":"","hash":"28d41c7b24128b63eed1ed71b77bc63e3f9b9b463af820a3171a0a1927b3f3a8"}
```

Só saídas JSON são verificáveis. Uma saída de texto mostra os 6 primeiros caracteres do hash, para achar a mesma entrada no JSON.

#### Configuração

```go
// Estado da cadeia num arquivo local
logger, _ := axio.New(config,
    axio.WithOutputs(axio.MustFile("/var/log/app.log", axio.FormatJSON)),
    axio.WithAudit("/var/lib/axio/chain.json"),
)
```

Todo Logger e Event auditado com o mesmo caminho num processo estende **uma** cadeia, qualquer que tenha sido criado primeiro.

Um Logger auditado precisa de uma saída JSON: só as linhas JSON carregam os hashes contra os quais o log é verificado, então `New` devolve `ErrAuditWithoutJSON` quando todas as saídas são de texto. Um Event escreve JSON em toda saída e não tem essa exigência.

A primeira escrita toma um lock exclusivo num arquivo ao lado do store (`chain.json.lock`) e o segura enquanto o processo roda: um segundo processo com o mesmo caminho falha no `New` com `ErrChainStoreLocked`, em vez de bifurcar a cadeia. Ler o store, como a verificação faz, não toma lock. O lock usa `flock`, então Windows, Solaris e AIX ficam sem ele.

#### Verificando um log

```go
chain, err := axio.NewHashChain(axio.NewFileStore("/var/lib/axio/chain.json"))
if err != nil {
    return err
}
file, err := os.Open("/var/log/app.log")
if err != nil {
    return err
}
defer file.Close()

// "" porque o arquivo começa a cadeia; num arquivo rotacionado, passe o
// último hash do arquivo anterior.
if err := chain.Verify(file, ""); err != nil {
    return fmt.Errorf("o log de auditoria não confere: %w", err)
}
```

| Erro                 | Significado                                                        |
| -------------------- | ------------------------------------------------------------------ |
| `ErrHashMismatch`    | O conteúdo de uma linha mudou depois de escrito                     |
| `ErrChainBroken`     | Uma linha foi removida, movida ou inserida, ou não tem os hashes no fim |
| `ErrChainIncomplete` | O log termina antes da cadeia: o fim foi apagado ou a cadeia inteira foi reescrita |

Para arquivos rotacionados, `axio.VerifyLines(reader, previousHash)` confere um arquivo e devolve o hash da sua última linha — o `previousHash` do arquivo seguinte. `chain.Verify` é `VerifyLines` mais a conferência de que o log termina no último hash da cadeia. No terminal, o `axio verify` faz as duas coisas (veja abaixo).

#### ChainStore Customizado

Implemente `ChainStore` para backends customizados (Redis, PostgreSQL, etc.) e passe a cadeia com `WithAuditChain`. Loggers e Events que recebem a mesma cadeia estendem uma cadeia só:

```go
type ChainStore interface {
    Save(sequence uint64, lastHash string) error
    Load() (sequence uint64, lastHash string, err error)
}

chain, err := axio.NewHashChain(redisStore)
if err != nil {
    return err
}
logger, _ := axio.New(config, axio.WithAuditChain(chain))
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
    HookDuration(ctx context.Context, hookName string, duration time.Duration, hasError bool)
}
```

---

### Wide Events

#### O que são Wide Events?

**Wide events** (também conhecidos como *canonical log lines*) substituem várias linhas de log espalhadas pela execução por uma única entrada ricamente anotada, emitida ao final de uma unidade de trabalho (por exemplo, uma requisição HTTP). Em vez de 5–10 linhas por requisição, um único evento captura todo o contexto do que aconteceu.

Wide events omitem o campo de nível — a severidade é expressa pelos próprios campos do evento (`status_code`, `error`, etc.), não por níveis tradicionais de log.

#### Uso Básico

```go
event, err := axio.NewEvent("checkout", config)
if err != nil {
    return err
}
defer event.Close()

event.Add("user_id", userID)
event.Add("cart_total", 15999)
event.Add("item_count", 3)

event.Emit(ctx)
// Saída: {"timestamp":"...","event":"checkout","user_id":"usr_456","cart_total":15999,"item_count":3,"duration_ms":42}
```

#### API

| Método | Descrição |
| ------ | --------- |
| `NewEvent(name, config, ...Option)` | Cria um novo evento usando o mesmo Config/Option de `New` |
| `Add(key, value)` | Adiciona um campo chave-valor (thread-safe) |
| `With(...Annotation)` | Adiciona anotações, incluindo tipos `Annotable` como `HTTP` |
| `SetError(err, ...Annotation)` | Registra um erro com anotações de detalhe opcionais |
| `Emit(ctx)` | Escreve o evento como uma única entrada (calcula `duration_ms`, executa hooks; um hook pode renomear o evento) |
| `Close()` | Libera recursos de saída (chame depois de `Emit`); um `Emit` depois do `Close` não escreve nada, e um segundo `Close` devolve `ErrEventClosed` |

#### Propagação via Context (padrão middleware)

Armazene o evento no contexto para que handlers downstream possam enriquecê-lo:

```go
// Middleware: cria e armazena
event, _ := axio.NewEvent("http_request", config)
defer event.Close()
ctx = axio.WithEvent(ctx, event)

// Handler: enriquece a partir do contexto
event := axio.EventFromContext(ctx)
event.Add("user_id", userID)
event.With(axio.Field("http", axio.HTTP{
    Method:     r.Method,
    URL:        r.URL.Path,
    StatusCode: 201,
    LatencyMS:  latencyMS,
}))

// Middleware: emite no final da requisição
event.Emit(ctx)
```

#### Registro de Erros

```go
// Erro simples
event.SetError(err)

// Erro com detalhes estruturados
event.SetError(err,
    axio.Field("error_code", "card_declined"),
    axio.Field("error_retriable", false),
)
```

#### Integração com Recursos

Wide events suportam as mesmas Options do logger padrão:

```go
// Com mascaramento de PII
event, _ := axio.NewEvent("user_registration", config,
    axio.WithPII(nil, nil),
)

// Com hash chain de auditoria
event, _ := axio.NewEvent("access_grant", config,
    axio.WithAudit("/var/lib/axio/chain.json"),
)

// Com tracing
event, _ := axio.NewEvent("http_request", config,
    axio.WithTracer(axio.Otel()),
)
```

---

## Linha de Comando: axio render e axio verify

```bash
go install github.com/pragmabits/axio/cmd/axio@latest
```

O comando é um módulo próprio, `github.com/pragmabits/axio/cmd/axio`, então importar a biblioteca não traz junto a dependência do Cobra.

### axio render

Logs feitos para máquinas são JSON. O `axio render` transforma esses logs de volta no texto que uma pessoa lê no terminal: o mesmo texto que a saída `Console` escreve, byte a byte, com as cores.

```bash
kubectl logs -f deploy/checkout | axio render
kubectl logs -f -l app=checkout --prefix | axio render
docker compose logs -f checkout | axio render
journalctl -u checkout -o cat -f | axio render
axio render /var/log/checkout.log
kubectl logs deploy/checkout | axio render | grep -E 'WARN|ERROR'
kubectl logs deploy/checkout | axio render --color=always | less -R
```

- Lê arquivos em ordem, ou a entrada padrão; `-` também indica a entrada padrão
- Escreve cada linha assim que a lê, então acompanha streams com `-f`
- Linhas que não são entradas do axio passam intactas; um prefixo antes do JSON (nome do pod, serviço do compose) fica na frente
- Wide events mostram `EVENT` na coluna de nível
- `--color=auto|always|never`: `auto` só colore um terminal e respeita `NO_COLOR`
- `--utc`: horários em UTC em vez do fuso local
- `axio completion bash|zsh|fish|powershell` gera o autocompletar do shell

### axio verify

Confere um log JSON auditado contra a cadeia guardada pelo `WithAudit`: o hash de cada linha confere com a linha, cada linha continua a anterior e o log termina onde a cadeia termina.

```bash
axio verify --store /var/lib/axio/chain.json /var/log/app.log
axio verify --store chain.json app.log.2 app.log.1 app.log          # arquivos rotacionados, do mais antigo ao mais novo
axio verify --store chain.json --previous-hash "$(tail -1 app.log.1 | jq -r .hash)" app.log
kubectl logs deploy/payments | axio verify --store chain.json
```

```text
verified: the log reaches the chain's last hash 550e01                      # saída 0
axio: app.log.1: hash mismatch: line 2                                      # saída 1
axio: app.log: chain integrity compromised: line 1 does not continue the line before it
axio: log does not reach the chain's last hash: log ends at "f0503d", chain at "550e01"
```

- `--store` é obrigatório; `--previous-hash` é o hash da linha anterior à primeira, quando o arquivo mais antigo que você tem não é o primeiro da cadeia
- Verifique um log que não está mais sendo escrito: enquanto um logger ainda escreve, a cadeia pode terminar depois da última linha lida

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

Para operações críticas, use `WithAudit` com uma saída JSON e combine com armazenamento confiável.

### 8. Campos HTTP padrão

```go
logger.Info(ctx, "requisição concluída",
    axio.Field("http", axio.HTTP{
        Method:     r.Method,
        URL:        r.URL.Path,
        StatusCode: statusCode,
        LatencyMS:  latencyMS,
        ClientIP:   r.RemoteAddr,
    }),
    axio.Field("request_id", requestID),
    axio.Field("user_id", userID),
)
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
logger.Info(ctx, "requisição finalizada", axio.Field("http", axio.HTTP{...}), axio.Field("request_id", id))
```

### Workers e Jobs

**Objetivo:** Saber quando iniciou, terminou, quanto processou.

| Evento        | Nível | Campos sugeridos                                             |
| ------------- | ----- | ------------------------------------------------------------ |
| Job iniciado  | Info  | `job_name`, `job_id`                                         |
| Job concluído | Info  | `+items_total`, `+items_ok`, `+items_failed`, `+duration_ms` |
| Erro em item  | Warn  | `+item_id`, `+error` (amostrado)                             |

```go
logger.Info(ctx, "job concluído",
    axio.Field("job_name", "reconcile_payments"),
    axio.Field("items_ok", okCount),
    axio.Field("items_failed", failedCount),
)
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
logger.Info(ctx, fmt.Sprintf("usuario=%s status=%d", userID, statusCode))
```

**Correto:**
```go
logger.Info(ctx, "requisição concluída",
    axio.Field("user_id", userID),
    axio.Field("status_code", statusCode),
)
```

### Anti-padrão: Payload com PII

**Errado:**
```go
logger.Info(ctx, fmt.Sprintf("payload=%+v", payload))
```

**Correto:**
```go
logger.Info(ctx, "payload recebido",
    axio.Field("payload_id", payload.ID),
    axio.Field("payload_size", len(payload.Data)),
)
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
    logger.Debug(ctx, "processando item", axio.Field("item_id", item.ID))
}
```

**Correto:**
```go
logger.Info(ctx, "lote processado",
    axio.Field("items_total", len(items)),
    axio.Field("items_ok", okCount),
    axio.Field("items_failed", failedCount),
)
```

### Anti-padrão: Cardinalidade explosiva

**Errado:**
```go
logger.Info(ctx, "login", axio.Field("email", user.Email))
```

**Correto:**
```go
logger.Info(ctx, "login", axio.Field("user_id", user.ID))
```

### Anti-padrão: Mensagem vaga

**Errado:**
```go
logger.Error(ctx, err, "erro")
```

**Correto:**
```go
logger.Error(ctx, err, "falha ao confirmar pagamento",
    axio.Field("order_id", order.ID),
)
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
| `ErrAuditWithoutJSON`    | Logger auditado só com saídas texto  | Adicione uma saída JSON; só JSON é verificável |
| `ErrInvalidPIIMaxDepth`  | Profundidade de PII negativa         | Use `0` para o padrão, ou uma profundidade positiva |
| `ErrInvalidTracer`       | Valor de TracerType inválido         | Use `otel` ou `noop`                         |
| `ErrLoadConfig`          | Falha ao ler arquivo de config       | Verifique caminho e permissões               |
| `ErrUnknownFormat`       | Extensão de arquivo desconhecida     | Use `.yaml`, `.yml`, `.json` ou `.toml`      |
| `ErrUnmarshalConfig`     | Falha ao fazer parse do config       | Verifique sintaxe do arquivo                 |
| `ErrApplyOption`         | Falha ao aplicar Option              | Verifique parâmetros da Option               |
| `ErrValidateConfig`      | Configuração inválida após Options   | Verifique combinação de valores              |
| `ErrBuildOutputs`        | Falha ao criar outputs               | Verifique caminhos de arquivo                |
| `ErrBuildHooks`          | Falha ao criar hooks                 | Verifique regex de PIICustomPatterns         |
| `ErrBuildMetrics`        | Falha ao construir métricas          | Verifique configuração do MeterProvider      |
| `ErrBuildAudit`          | Falha ao construir a cadeia de audit | Verifique o caminho do store e as permissões |
| `ErrBuildEngine`         | Falha ao construir engine de logging | Verifique combinação de outputs e config     |
| `ErrOpenFile`            | Falha ao abrir arquivo de log        | Verifique caminho e permissões               |
| `ErrOutputClosed`        | Output de arquivo fechado duas vezes | Feche cada output uma vez                    |
| `ErrLoadChainState`      | Falha ao carregar estado da chain    | Verifique arquivo de chain                   |
| `ErrSaveChainState`      | Falha ao salvar estado da chain      | Verifique permissões de escrita              |
| `ErrMarshalChainState`   | Falha ao serializar estado da chain  | Erro interno de serialização                 |
| `ErrUnmarshalChainState` | Falha ao desserializar estado da chain | Arquivo de chain corrompido ou formato inválido |
| `ErrHashMismatch`        | O hash de uma linha não confere      | A linha mudou depois de escrita              |
| `ErrChainBroken`         | Integridade da cadeia comprometida   | Uma linha foi removida, movida ou inserida   |
| `ErrChainIncomplete`     | O log termina antes da cadeia        | Fim apagado ou cadeia inteira reescrita      |
| `ErrNilAuditChain`       | Cadeia passada a WithAuditChain é nil| Passe uma cadeia de `NewHashChain`           |
| `ErrChainStoreLocked`    | Outro processo segura o store        | Um processo por store; dê a cada um seu caminho |
| `ErrNilMetricsProvider`  | Provider de métricas é nil           | Passe um MeterProvider válido                |
| `ErrCreateMetric`        | Falha ao criar instrumento OTel      | Verifique configuração do provider           |
| `ErrNilTracer`           | Tracer passado a WithTracer é nil    | Passe um Tracer não-nulo ou omita a opção    |
| `ErrLoggerClosed`        | Logger já foi fechado                | Guarda idempotente; cheque com `errors.Is`   |
| `ErrLoggerNotRoot`       | Close chamado em um Logger derivado  | Apenas o root retornado por `New` pode fechar|
| `ErrEventClosed`         | Event já foi fechado                 | Guarda idempotente; cheque com `errors.Is`   |
