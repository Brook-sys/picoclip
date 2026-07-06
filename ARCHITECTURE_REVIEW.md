O projeto possui uma arquitetura de orquestração local de agentes sólida baseada em Go, utilizando `cmd/picoclip` como entrypoint e `internal/core` como camada de domínio.

Para "revisar a arquitetura" de forma efetiva, proponho os seguintes pontos críticos de evolução baseados na observação do `internal/core/services/runner.go` e `engine.go`:

1.  ** desacoplamento da montagem de prompts**: O `Runner` é responsável por montar prompts (linhas 84-106 de `runner.go`), o que está inflando a lógica do serviço. Devemos mover isso para um `PromptBuilder` ou serviço dedicado.
2.  **Persistência de eventos e logs**: Atualmente, o sistema de eventos é em memória e falta streaming real. É necessário criar um repositório de logs e persistir eventos de forma assíncrona.
3.  **Cancelamento robusto**: A issue mencionada em `AGENTS.md` sobre cancelamento de execução precisa ser tratada integrando o `cancel()` do contexto com o driver de execução, permitindo que o `Runner` sinalize o driver a interromper o processo externo.
4.  **Refinamento do Driver Registry**: O `Runner` conhece detalhes de drivers (verificar `runtimes` no `runner.go`). Poderíamos generalizar a execução via interface de Driver mais estrita.

Gostaria de focar em qual desses pontos primeiro? Posso começar implementando o `PromptBuilder` para reduzir a carga do `Runner`.
