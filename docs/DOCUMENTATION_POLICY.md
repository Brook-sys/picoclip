# Política de Documentação do PicoClip

Esta política define como a documentação do PicoClip deve evoluir junto com o código. Ela vale para humanos e para agentes de IA trabalhando no repositório.

## Objetivo

Qualquer pessoa ou agente deve conseguir responder rapidamente:

- o que o PicoClip faz;
- como o projeto está organizado;
- como rodar, testar e validar;
- onde uma funcionalidade vive no código;
- quais APIs, páginas e fluxos existem;
- qual é o estado real do projeto hoje;
- quais limitações ainda existem;
- qual documentação precisa ser atualizada quando algo muda.

Documentação no PicoClip não é acabamento. Ela é parte do produto e parte da confiabilidade do projeto.

## Princípios

1. **Documentar o estado real, não o desejado**
   - Se algo ainda é plano, escreva como plano.
   - Se algo é parcial, escreva como parcial.
   - Se algo mudou no código, atualize a documentação na mesma mudança sempre que possível.

2. **Preferir documentação operacional**
   - Use comandos reais do `Makefile`, scripts e código.
   - Explique como verificar, diagnosticar e recuperar.
   - Inclua limitações e riscos conhecidos.

3. **Manter um mapa para orientação rápida**
   - O ponto de entrada é o `README.md` / `README.pt-BR.md`.
   - Agentes devem começar por `AGENTS.md` e `docs/PROJECT_MAP.md`.
   - Detalhes por área ficam em `docs/`.

4. **Evitar documentação duplicada sem necessidade**
   - Quando um assunto já tem documento canônico, linke para ele.
   - Só duplique resumos curtos quando isso melhora a navegação.

5. **Manter português e inglês alinhados quando houver pares**
   - `README.md` e `README.pt-BR.md` devem ter a mesma estrutura e links principais.
   - Documentos sem par podem ficar em português enquanto o projeto estiver evoluindo rápido.

6. **Toda funcionalidade nova precisa ter rastro documental proporcional**
   - Pequenas correções podem exigir apenas changelog interno/nota no doc existente.
   - Novos fluxos, APIs, status, permissões, runtimes, storage, UI ou operações precisam de documentação explícita.

## Camadas de documentação

| Documento | Função | Quando atualizar |
| --- | --- | --- |
| `README.md` / `README.pt-BR.md` | Visão geral, quick start, links principais | Mudanças de posicionamento, instalação, status geral, links |
| `AGENTS.md` | Guia operacional para agentes de IA no repo | Mudanças em comandos, arquitetura, prioridades, regras de trabalho |
| `docs/PROJECT_MAP.md` | Mapa do código, módulos, fluxos e onde procurar | Sempre que criar/mover módulos, páginas, serviços ou adapters |
| `docs/DEVELOPMENT.md` | Workflow local, comandos, testes, debugging | Mudanças em Makefile, scripts, ferramentas, validação |
| `docs/API_REFERENCE.md` | APIs admin, Agent API e rotas relevantes | Novos endpoints, payloads, permissões, breaking changes |
| `docs/ROBUSTNESS.md` / `.pt-BR.md` | Recovery, retry, cancellation, liveness | Mudanças em scheduler, dispatcher, runner, reconciler, locks, wakeups |
| `docs/STORAGE.md` | SQLite/memory, migrations, backup/restore | Mudanças em schema, repositories, restore, storage config |
| `docs/DESIGN.md` | UI, componentes, HTMX, padrões visuais | Novas páginas, componentes, padrões de interação |
| `docs/CURRENT_STATE.md` | Estado real consolidado | Após marcos grandes ou mudanças de arquitetura/semântica |
| `docs/ROADMAP.md` | Próximas fases e critérios de aceite | Ao concluir entregas ou mudar prioridades |

## Checklist obrigatório para mudanças

Use este checklist em todo PR, commit grande ou tarefa feita por agente.

### Para qualquer mudança de código

- [ ] Existe teste ou validação proporcional?
- [ ] Algum comando, configuração ou fluxo de execução mudou?
- [ ] Algum status, evento, permissão, endpoint, payload ou tabela mudou?
- [ ] Alguma limitação documentada ficou obsoleta?
- [ ] Algum documento canônico precisa de atualização?

### Para nova funcionalidade

Atualize pelo menos um destes:

- [ ] `docs/PROJECT_MAP.md` se criou/moveu área ou fluxo importante.
- [ ] `docs/API_REFERENCE.md` se expôs ou alterou endpoint.
- [ ] `docs/DEVELOPMENT.md` se mudou workflow de dev/teste.
- [ ] `docs/DESIGN.md` se mudou UI ou padrão HTMX.
- [ ] `docs/STORAGE.md` se mudou schema/storage/migration.
- [ ] `docs/ROBUSTNESS.md` se mudou retry/recovery/cancelamento/liveness.
- [ ] `docs/CURRENT_STATE.md` se mudou o estado macro do projeto.
- [ ] `docs/ROADMAP.md` se concluiu ou repriorizou entrega.

### Para bugfix de robustez/confiabilidade

- [ ] O teste de regressão falhou antes do fix?
- [ ] O documento de robustez descreve o comportamento atual?
- [ ] Eventos, logs ou diagnostics relevantes estão documentados?
- [ ] O runbook de investigação continua correto?

### Para nova API ou alteração de API

- [ ] Endpoint, método e path adicionados em `docs/API_REFERENCE.md`.
- [ ] Payload de request/response documentado quando não for óbvio.
- [ ] Permissão/capability necessária documentada.
- [ ] Alias `tasks`/`issues` documentado quando existir.
- [ ] Exemplo `curl` adicionado se o endpoint for importante para agentes.

### Para nova página ou componente de UI

- [ ] Página listada em `docs/PROJECT_MAP.md`.
- [ ] Padrão visual/HTMX documentado em `docs/DESIGN.md` se for reutilizável.
- [ ] E2E smoke adicionado ou justificado.

## Matriz de validação mínima por tipo de mudança

Use esta matriz como ponto de partida. Ela define o mínimo esperado por tipo de
mudança, mas não substitui julgamento: se a alteração toca várias áreas, combine as
linhas relevantes. `make check` continua sendo a validação canônica antes de
concluir mudanças relevantes de código, UI, API, storage, runtime ou documentação
que altera comandos/contratos.

| Tipo de mudança | Documentos obrigatórios a revisar | Validação mínima | Observações |
| --- | --- | --- | --- |
| Docs-only | Documento editado, links no `README.md`/`AGENTS.md` se a navegação mudar | Verificação de links internos; `make check` quando o doc altera comandos, contratos ou arquivos usados por testes | Não documente comportamento futuro como atual. |
| UI / Templ / CSS | `docs/DESIGN.md`; `docs/PROJECT_MAP.md` se houver página/componente novo | Teste focado quando existir; `make templ-generate`; `make check` | Inclua E2E smoke ou justificativa quando criar fluxo/página nova. |
| API / Agent API | `docs/API_REFERENCE.md`; `docs/PROJECT_MAP.md` se houver novo grupo/handler; `docs/OPERATIONS.md` se afetar runbook | Teste focado do handler/contrato; `go test ./...`; `make check` | Documente método, path, payload, permissões/capabilities e exemplos agent-facing quando útil. |
| Storage / migrations | `docs/STORAGE.md`; `docs/PROJECT_MAP.md` se criar/mover adapter/repository | Contract test ou teste focado de repository/migration; `go test ./...`; `make check` | Mantenha paridade SQLite/memory quando aplicável e preserve restore/backup. |
| Robustez / retry / recovery | `docs/ROBUSTNESS.md` e, quando aplicável, `docs/ROBUSTNESS.pt-BR.md`; `docs/OPERATIONS.md` para runbooks | Teste focado reproduzindo o caso; `go test ./...`; `make check` | Bugfixes precisam de teste RED antes do fix e evidência de recovery/cancelamento. |
| Workflow dev / comandos | `docs/DEVELOPMENT.md`; `AGENTS.md`; `README.md`/`README.pt-BR.md` se afetar onboarding | Conferir comandos no `Makefile`/scripts; rodar o comando afetado quando seguro; `make check` se a mudança toca build/teste | Não invente comandos; registre limitações locais como Alpine/Playwright quando relevantes. |
| Roadmap / current-state | `docs/CURRENT_STATE.md`; `docs/ROADMAP.md`; docs de área afetada se o estado real mudou | Verificação de links internos; teste/validação que comprova a entrega citada quando houver código associado | Separe claramente entregue, parcial e planejado. |

## Política para agentes de IA

Agentes trabalhando no PicoClip devem seguir esta ordem:

1. Ler `AGENTS.md`.
2. Ler `docs/PROJECT_MAP.md` para se localizar.
3. Ler o documento específico da área alterada.
4. Verificar o estado real no código antes de editar documentação.
5. Atualizar docs no mesmo conjunto de mudanças sempre que o comportamento mudar.
6. Rodar validação proporcional e reportar comandos reais executados.

Agentes não devem:

- inventar comandos que não existem;
- documentar comportamento futuro como atual;
- apagar notas de limitação sem confirmar no código/testes;
- ignorar README/AGENTS quando uma mudança afeta orientação geral;
- commitar artefatos locais, screenshots ou bancos de dados por acidente.

## Estilo recomendado

- Use títulos claros e estáveis para facilitar links.
- Prefira tabelas para variáveis, endpoints, módulos e status.
- Use exemplos pequenos e executáveis.
- Escreva limitações com honestidade.
- Mantenha comandos em blocos `sh`, `bash`, `text`, `json` ou `powershell` conforme o caso.
- Para documentos longos, comece com resumo e índice.

## Validação de documentação

Antes de finalizar mudanças grandes de documentação, rode pelo menos:

```sh
make check-docs
```

Esse alvo valida links Markdown locais em `README.md`, `README.pt-BR.md`,
`AGENTS.md` e `docs/*.md`, incluindo fragmentos de âncora como
`#matriz-de-validação-mínima-por-tipo-de-mudança` contra headings reais. O script
usa apenas Python standard library e também roda no começo de `make check`.

Se a mudança tocar comandos, build, templates ou testes, rode também a validação canônica:

```sh
make check
```

## Regra de ouro

Se uma pessoa ou agente precisaria perguntar "onde isso fica?", "como valida?" ou "esse comportamento é real?", a documentação ainda não está boa o suficiente.
