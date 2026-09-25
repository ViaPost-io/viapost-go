# ViaPost Go SDK

SDK oficial, server-side, para a API pública do ViaPost. A versão `v0.5.0` cobre envio de e-mails,
mensagens e métricas, domínios, templates, webhooks, automações e consumo mensal.

> Nunca coloque uma API key no frontend, em logs ou no repositório. Leia a chave de um secret ou
> variável de ambiente do servidor.

## Instalação

```bash
go get github.com/ViaPost-io/viapost-go@v0.5.0
```

O SDK requer Go 1.25 ou posterior. Desenvolvimento, CI e releases usam o toolchain
Go 1.26.6 ou posterior para incluir as correções de segurança da biblioteca padrão.

## Quickstart

```go
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	viapost "github.com/ViaPost-io/viapost-go"
)

func main() {
	client, err := viapost.NewClient(os.Getenv("VIAPOST_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.Email.Send(ctx, viapost.SendRequest{
		From:    "hello@example.com",
		To:      []string{"customer@example.net"},
		Subject: "Olá pelo ViaPost",
		HTML:    "<strong>Funcionou!</strong>",
		Stream:  viapost.StreamTransactional,
	}, viapost.WithIdempotencyKey("order-123-welcome"))
	if err != nil {
		var apiErr *viapost.APIError
		if errors.As(err, &apiErr) {
			log.Printf("ViaPost HTTP %d: %s (request_id=%s)", apiErr.StatusCode, apiErr.Message, apiErr.RequestID)
		}
		log.Fatal(err)
	}

	log.Printf("mensagens aceitas: %d", len(result.Accepted))
}
```

Um programa executável está em [`examples/send`](./examples/send).

## Configuração

`NewClient` usa autenticação Bearer, `https://api.viapost.io`, timeout de 60 segundos e o
User-Agent `viapost-go/0.5.0`. Use `WithBaseURL`, `WithTimeout`, `WithUserAgent`,
`WithHTTPClient` ou `WithMaxRawResponseBytes` para customizar. Todo método recebe
`context.Context`; o primeiro limite atingido
entre o contexto e o timeout do cliente encerra a requisição. Mutações não são repetidas
automaticamente.

Por segurança, endpoints remotos exigem HTTPS; HTTP é aceito apenas para `localhost` e IPs de
loopback em desenvolvimento. O cliente não segue redirects, não envia cookies, limita respostas
JSON de sucesso a 8 MiB e mensagens RFC822/exports CSV a 40 MiB por padrão, e clona cada request
antes de adicionar headers do SDK. O limite de conteúdo bruto pode ser reduzido (ou ampliado até
128 MiB) com `WithMaxRawResponseBytes` sem alterar os limites de JSON e de erros.

A camada principal oferece:

- `Email.Send`
- `Messages.List`, `Get`, `Events`, `Metrics`, `Engagement` e `Timeseries`
- `Domains`, `Templates`, `Webhooks` e `Automations`
- `Usage.Get`

Use `client.Raw()` somente quando precisar de uma operação ainda não promovida à camada
ergonômica. Esse cliente de baixo nível é gerado diretamente do OpenAPI e seus nomes podem mudar
com o contrato.

As operações anônimas da página pública usam um cliente dedicado para que a API key nunca seja
enviada ao host de status:

```go
statusClient, err := viapost.NewPublicStatusClient()
if err != nil {
	log.Fatal(err)
}
snapshot, err := statusClient.GetPublicStatus(ctx)
```

O cliente público bloqueia operações autenticadas e usa `https://status.viapost.io` por padrão.

## Contrato e geração

O arquivo [`openapi.yaml`](./openapi.yaml) é um snapshot versionado do contrato público em
[`docs.viapost.io/openapi/public.yaml`](https://docs.viapost.io/openapi/public.yaml), sincronizado
em 2026-09-25. O workflow agendado de drift verifica semanticamente o snapshot contra esse
contrato canônico.

SHA-256 do snapshot:

```text
7c931b5a4a2a602d3c42341f2a70af9c49378600894b31adebfd333469b9e183
```

O código em `api/` é gerado com ogen `v1.24.0`, está versionado e não busca schemas remotos:

```bash
make generate
make check-generated
```

O check de drift regenera o cliente a partir do snapshot local e verifica o diff. Comparações com
o contrato publicado devem ser semânticas, pois serializações YAML equivalentes podem ter hashes
textuais diferentes. Um workflow agendado faz essa comparação semântica contra o contrato canônico,
sem adicionar dependência de rede aos builds normais. Durante a geração, autenticação por cookie de
sessão e parâmetros CSRF administrativos são removidos da representação gerada: este SDK público é
exclusivamente autenticado por API key Bearer.

## Desenvolvimento e segurança

Consulte [CONTRIBUTING.md](./CONTRIBUTING.md), [SECURITY.md](./SECURITY.md) e
[CHANGELOG.md](./CHANGELOG.md). Licenciado sob a [MIT License](./LICENSE).

---

## English

The official server-side Go SDK for the ViaPost public API. Install it with:

```bash
go get github.com/ViaPost-io/viapost-go@v0.5.0
```

Create a client with `viapost.NewClient(os.Getenv("VIAPOST_API_KEY"))`, then call the resource
services listed above. The client uses Bearer authentication, a 60-second default timeout, honors
`context.Context`, returns typed `*viapost.APIError` values discoverable with `errors.As`, and does
not automatically retry mutations. Never expose an API key in client-side code.

`APIError.Body` and `APIError.Header` can contain request-related data. Redact them before sending
errors to shared logs or third-party observability systems.

The checked-in OpenAPI snapshot and generated client make builds and drift checks reproducible
without downloading a remote schema. See [CONTRIBUTING.md](./CONTRIBUTING.md) for the complete
verification workflow.
