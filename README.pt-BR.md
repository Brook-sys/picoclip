# PicoClip

_Leia em [Inglês / English](README.md)._

PicoClip é um motor leve e local-first de orquestração de agentes. Ele nasceu como uma alternativa inspirada no Paperclip, com foco adicional em **leveza, extrema portabilidade e consumo mínimo de recursos de hardware**.

A ideia é oferecer projetos/workspaces, agentes, tasks, runs, mensagens, delegação, permissões, skills e APIs para agentes, mantendo o core simples, pequeno e fácil de executar em praticamente qualquer lugar.

## Aviso importante: vibe coding

O PicoClip atualmente é escrito inteiramente através de **vibe coding**, com desenvolvimento fortemente assistido por IA.

Por causa disso:

- use o projeto com cuidado;
- desencorajamos fortemente o uso em produção;
- arquitetura, APIs, fluxos de UI e detalhes de implementação podem mudar rapidamente;
- partes grandes do código podem ser reescritas ou reorganizadas conforme o projeto evolui;
- revise o código antes de executar em ambientes sensíveis.

Isso não significa que o projeto é feito de forma descuidada. Significa que ele é experimental, evolui rápido e é intencionalmente transparente sobre como está sendo construído.

## Por que PicoClip?

PicoClip é inspirado na ideia de orquestração local de agentes, mas tenta permanecer extremamente pequeno e prático.

Os principais objetivos são:

- binário Go pequeno;
- baixo uso de RAM;
- operação local-first;
- arquitetura modular simples;
- drivers plugáveis;
- storage plugável;
- UI leve renderizada no servidor com HTMX e Templ;
- APIs úteis para humanos e agentes;
- permissões e capacidades reais em vez de cargos meramente decorativos;
- skills reutilizáveis como pacotes de instrução/contexto.

## Estado atual

PicoClip está em desenvolvimento ativo.

Ele já inclui uma UI web funcional, persistência SQLite, gerenciamento de ciclo de vida de tasks, agentes, skills, projetos/workspaces, adapters de runtime, APIs administrativas locais, uma Agent API para fluxos conduzidos por agentes, diagnostics, eventos Activity/SSE, suporte a cancelamento, recuperação de locks, detecção de runs travados e retry agendado com backoff.

Mesmo assim, ainda é cedo. Alguns comportamentos continuam sendo refinados, e partes do sistema podem mudar bastante com o tempo. As áreas de robustez mais importantes agora são classificação de retry, visibilidade de recovery, enforcement de permissões, streaming de eventos de runtime e dashboards operacionais.

## Como o PicoClip funciona

PicoClip é construído em torno de um ciclo pequeno de orquestração:

1. humanos ou agentes criam tasks;
2. tasks são atribuídas a agentes e ficam executáveis;
3. o dispatcher reivindica tasks executáveis com metadata de checkout/lock;
4. o runner executa a task através de um runtime adapter;
5. runs produzem eventos, mensagens, output, erros e metadata de uso;
6. o reconciler repara locks antigos, processa wakeups, detecta runs travados e agenda retry quando apropriado.

O sistema é intencionalmente local-first: o storage padrão é um banco SQLite local, workspaces ficam no filesystem local e adapters de runtime são comandos locais.

## Modelo de robustez

PicoClip tenta falhar de forma visível e recuperar de forma conservadora. Recursos atuais de confiabilidade incluem:

- persistência SQLite por padrão;
- checkout atômico de task e locks de execução;
- recuperação de locks antigos;
- cancelamento de runtime via adapters;
- detecção de run travado baseada em heartbeat/output;
- wakeups de retry com backoff exponencial;
- eventos Activity `retry.scheduled`, `run.timeout` e `run.recovered`;
- diagnostics para storage, runtime paths, workspace paths e runtimes configurados.

Veja [Robustez, Recovery e Aprendizado com Falhas](docs/ROBUSTNESS.pt-BR.md) para o modelo operacional detalhado.

## Como iniciar rapidamente

PicoClip é distribuído como um binário único. Ele não exige banco de dados externo nem serviços pesados em tempo de execução.

### Opção 1: Rodar um binário pronto

Baixe o binário mais recente na página de [GitHub Releases](https://github.com/Brook-sys/picoclip/releases), escolhendo o arquivo correto para sua plataforma.

Exemplo para Linux x64:

```sh
tar -xzf picoclip-v0.0.1-linux-amd64.tar.gz
chmod +x picoclip-v0.0.1-linux-amd64
./picoclip-v0.0.1-linux-amd64
```

Exemplo para macOS Apple Silicon:

```sh
tar -xzf picoclip-v0.0.1-darwin-arm64.tar.gz
chmod +x picoclip-v0.0.1-darwin-arm64
./picoclip-v0.0.1-darwin-arm64
```

Exemplo para Windows:

```powershell
Expand-Archive picoclip-v0.0.1-windows-amd64.zip
.\picoclip-v0.0.1-windows-amd64\picoclip-v0.0.1-windows-amd64.exe
```

Depois abra:

```text
http://127.0.0.1:8088
```

Por padrão, o PicoClip escuta em `0.0.0.0:8088`. Você pode mudar isso com:

```sh
BIND=127.0.0.1 PORT=9090 ./picoclip-v0.0.1-linux-amd64
```

### Opção 2: Rodar com Docker / Podman

Imagem padrão baseada em Alpine:

```sh
docker run --rm -p 8088:8088 \
  -v picoclip-data:/app/data \
  -v picoclip-workspaces:/app/workspaces \
  ghcr.io/brook-sys/picoclip:latest
```

Se você precisa do runtime Claurst, use a variante Debian/glibc, porque o binário Linux oficial do Claurst não roda de forma confiável em Alpine/musl:

```sh
docker run --rm -p 8088:8088 \
  -v picoclip-data:/app/data \
  -v picoclip-workspaces:/app/workspaces \
  ghcr.io/brook-sys/picoclip:latest-debian
```

Depois abra:

```text
http://127.0.0.1:8088
```

Você também pode usar `podman run` com os mesmos argumentos.

### Opção 3: Rodar pelo código fonte

Requisitos:

- Go
- Git

```sh
git clone https://github.com/Brook-sys/picoclip.git
cd picoclip
make tools
make run
```

Depois abra:

```text
http://127.0.0.1:8088
```

Dados de demonstração opcionais:

```sh
make seed
```

Configuração útil em runtime:

| Variável | Padrão | Finalidade |
| --- | --- | --- |
| `BIND` | `0.0.0.0` | Endereço de bind HTTP. Use `127.0.0.1` para acesso somente local. |
| `PORT` | `8080` no binário, `8088` no Makefile | Porta HTTP. |
| `PICOCLIP_STORAGE` | `sqlite` | `sqlite` ou `memory`. Use `memory` somente para sessões/testes temporários. |
| `PICOCLIP_DB_PATH` | `data/picoclip.db` | Caminho do banco SQLite. |
| `PICOCLIP_WORKSPACES` | `workspaces` | Diretório base dos projetos/workspaces. |
| `PICOCLIP_RUNTIMES` | `data/runtimes` | Diretório base do estado dos runtimes. |
| `PICOCLIP_LOG_LEVEL` | `info` | Nível de log. |
| `PICOCLIP_DEBUG` | `false` | Ativa comportamento de debug quando `true` ou `1`. |
| `CRUSH_PATH` | `crush` | Executável do runtime Crush. |
| `PICOCLAW_PATH` | `picoclaw` | Executável do runtime PicoClaw. |
| `CLAURST_PATH` | `claurst` | Executável do runtime Claurst. |

Modo de desenvolvimento com live reload:

```sh
make dev
```

Build local:

```sh
make build
./picoclip
```

Validar tudo:

```sh
make check
```

## Roadmap

Existe um roadmap ativo, e mais recursos serão adicionados gradualmente conforme o projeto amadurece.

Veja:

- [Mapa do Projeto](docs/PROJECT_MAP.md)
- [Política de Documentação](docs/DOCUMENTATION_POLICY.md)
- [Referência de API](docs/API_REFERENCE.md)
- [Runbook Operacional](docs/OPERATIONS.md)
- [Roadmap](docs/ROADMAP.md)
- [Estado Atual](docs/CURRENT_STATE.md)
- [Arquitetura de Storage](docs/STORAGE.md)
- [Robustez, Recovery e Aprendizado com Falhas](docs/ROBUSTNESS.pt-BR.md)
- [Guia de Desenvolvimento](docs/DEVELOPMENT.md)
- [Design System](docs/DESIGN.md)

## Colaboração

Colaborações são muito bem-vindas.

Este projeto tem espírito open-source e está aberto a críticas, reports de bugs, sugestões de funcionalidades, feedback arquitetural e pull requests.

Você pode ajudar:

- abrindo issues para bugs;
- sugerindo novas funcionalidades;
- revisando decisões de design;
- melhorando documentação;
- testando o projeto em diferentes ambientes;
- enviando pull requests.

Como este é um projeto feito via vibe coding, feedback externo é especialmente valioso. Ele ajuda a manter o projeto mais pé no chão, útil e mais seguro para evoluir.

## Uso em produção

PicoClip **não é recomendado para uso em produção** neste momento.

Se ainda assim você decidir executá-lo, trate como software experimental e revise cuidadosamente o comportamento do sistema.
