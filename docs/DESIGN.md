# PicoClip Design System

PicoClip usa um design system leve, server-rendered e local-first construído com Templ, HTMX, JavaScript pequeno e CSS puro. O objetivo é manter a UI consistente, operável e fácil de estender sem transformar o projeto em uma SPA pesada.

Leia junto com:

- [Project Map](PROJECT_MAP.md) — onde vivem handlers, templates, rotas e assets.
- [Development Guide](DEVELOPMENT.md) — workflow Templ, validação e E2E.
- [API Reference](API_REFERENCE.md) — rotas web, partials, SSE e endpoints usados pela UI.
- [Operations Runbook](OPERATIONS.md) — padrões operacionais que a UI deve expor com clareza.
- [Documentation Policy](DOCUMENTATION_POLICY.md) — quando atualizar este documento.

## Objetivos

A UI do PicoClip deve:

- ser rápida e simples de renderizar no servidor;
- funcionar bem em um binário Go local;
- usar HTMX apenas para interações e regiões live bem delimitadas;
- expor falhas, runs, retries e diagnostics de forma operacional;
- manter formulários HTML normais e progressivamente aprimorados;
- evitar dependências visuais pesadas;
- manter componentes reutilizáveis em vez de markup ad-hoc;
- permanecer navegável para humanos e agentes.

## Princípios

1. **Server-rendered first**
   - O HTML inicial deve vir do servidor.
   - Templ é a fonte dos templates.
   - JavaScript melhora interações, mas não deve ser requisito para entender a página.

2. **Componentes antes de variações locais**
   - Use `ui.templ` antes de criar classes ou blocos novos.
   - Se uma forma visual aparece duas vezes, considere componente reutilizável.

3. **HTMX com escopo pequeno**
   - Use HTMX para forms, ações e partials.
   - Evite atualizar `<body>` por polling.
   - Polling deve mirar regiões pequenas em `/partials/...`.

4. **Observabilidade visível**
   - Estados de erro, timeout, retry e recovery devem aparecer com badges, activity e run details.
   - Dashboard deve priorizar o que exige atenção humana.

5. **Acessibilidade básica sempre**
   - Botões icon-only precisam de `aria-label`.
   - Forms precisam de labels.
   - Modais devem focar primeiro controle e fechar com Escape.
   - Feedback assíncrono global deve usar uma região live (`role="status"`, `aria-live="polite"`, `aria-atomic="true"`).
   - Forms HTMX em andamento devem marcar `aria-busy="true"` e remover o atributo ao concluir.
   - Estados destrutivos devem ser claros visualmente e semanticamente.

6. **Sem segredos na UI/docs**
   - Valores sensíveis de env vars devem ser mascarados por padrão.
   - Exemplos em documentação devem usar placeholders.

## Arquivos principais

| Arquivo | Papel |
| --- | --- |
| `internal/adapters/web/layout.templ` | Shell principal, navegação, tema, command palette, JS global e custom elements. |
| `internal/adapters/web/ui.templ` | Componentes reutilizáveis: botões, cards, forms, badges, tabs, rows, empty states. |
| `internal/adapters/web/icons.templ` | Ícones SVG inline. |
| `internal/adapters/web/modals.templ` | Modais reutilizáveis. |
| `internal/adapters/web/*_page.templ` | Páginas de lista/área. |
| `internal/adapters/web/*_detail.templ` | Páginas de detalhe. |
| `internal/adapters/web/assets/app.css` | Tokens, layout, componentes, padrões de página e responsividade. |
| `internal/adapters/web/server.go` | Registro de rotas web, APIs, partials e SSE. |

## Shell e navegação

O shell principal é `PageShell(active, title, subtitle, action)` em `layout.templ`.

Ele fornece:

- HTML base;
- carregamento do HTMX local em `/assets/htmx.min.js`;
- CSS em `/assets/app.css`;
- sidebar;
- header padrão de página;
- slot de ação da página;
- command palette;
- toast root;
- script global de interações.

A navegação é agrupada por área:

| Grupo | Páginas |
| --- | --- |
| Control | Dashboard, Tasks, Runs, Activity |
| Core | Projects, Agents, Skills |
| Admin | Settings |

Regra: novas páginas devem entrar em um grupo existente ou justificar um novo grupo. Não adicione links aleatórios na sidebar.

## Tema e densidade

O shell aplica preferências antes do CSS principal para evitar flash visual:

- tema salvo em `localStorage['picoclip.theme']`;
- valores: `system`, `light`, `dark`;
- tema resolvido em `document.documentElement.dataset.theme`;
- densidade salva em `localStorage['picoclip.density']`;
- densidade aplicada em `document.documentElement.dataset.density`.

Tokens de tema vivem em `app.css`:

- `:root` para tema claro;
- `[data-theme="dark"]` para tema escuro;
- variáveis legadas continuam mapeadas para migração gradual.

## Modelo de tokens CSS

`app.css` usa três camadas principais.

### Primitive tokens

Exemplos:

- `--a2-neutral-*`
- `--a2-blue-*`
- `--a2-green-*`
- `--a2-red-*`
- `--a2-yellow-*`

### Semantic tokens

Exemplos:

- `--a2-background-color-primary`
- `--a2-surface-color`
- `--a2-surface-hover`
- `--a2-foreground-color-primary`
- `--a2-foreground-color-secondary`
- `--a2-border-color-default`
- `--a2-border-color-hover`
- `--a2-focus-color`

### PicoClip identity tokens

A identidade visual usa uma base precisa inspirada em ferramentas de desenvolvimento dark-mode-first, mas continua leve e local-first. Tokens explícitos vivem em `app.css` para evitar cores espalhadas:

- `--brand` e `--brand-strong` definem o indigo/violet principal do produto;
- `--brand-soft` aplica tintas sutis em ícones, headers e estados ativos;
- `--brand-ring` e `--focus-ring` padronizam foco e contornos acessíveis;
- `--surface-gradient` dá profundidade consistente para `card`, `panel` e `metric-card`;
- `--surface-overlay`, `--surface-raised`, `--text-strong`, `--text-muted` e `--shadow-glow` refinam contraste e profundidade do modo escuro sem espalhar cores locais.

Regras:

- use `--brand*` apenas para identidade, foco e affordances interativas importantes;
- use `--surface-gradient` para superfícies canônicas em vez de gradientes locais diferentes;
- use `--surface-raised` em componentes que precisam se destacar do canvas escuro (`.pc-card`, botões secundários, ícones de header) e `--surface-overlay` em controles/editors;
- mantenha texto em tokens semânticos (`--text`, `--muted`, `--text-strong`, `--text-muted`) para preservar contraste entre temas;
- mudanças nestes tokens devem manter `TestDesignSystemCSSDefinesDarkModeDepthAndContrastTokens` verde.

### Layout/shape tokens

Exemplos:

- spacing: `--space-1` até `--space-8`;
- radius: `--radius-sm`, `--radius-md`, `--radius-lg`;
- elevation: `--shadow-sm`, `--shadow-md`;
- actions: `--button-shadow` e `--button-hover-transform`.

Regra: ao criar CSS novo, use tokens existentes antes de codificar cores/espaçamentos literais.

### Action/button identity

A UI ainda convive com a classe legada `.button` e os helpers novos `.pc-btn*`. Para evitar duas aparências concorrentes, ambas as famílias devem compartilhar a mesma linguagem visual e toda área tocada deve migrar para helpers/classes canônicas:

- ações primárias usam `linear-gradient(135deg, var(--brand), var(--brand-strong))`, `--brand-ink` e `--button-shadow`;
- ações secundárias usam `--surface-gradient`, borda semântica e `--shadow-sm`;
- hover vertical usa `--button-hover-transform`, sem animações pesadas;
- links de ação devem preferir `ButtonLink`; botões simples devem usar `.pc-btn pc-btn-*` ou ganhar um helper Templ quando precisarem de atributos extras;
- `IconButton`/`.pc-icon-btn` deve parecer superfície elevada, não botão primário;
- mudanças nos botões devem manter `TestDesignSystemCSSDefinesConsistentActionButtons` verde; o teste verifica declarações CSS por seletor/propriedade em vez de depender de strings longas de formatação;
- quando migrarem uma página, adicionar/ajustar teste de regressão contra `class="button`.

### Form-control identity

Inputs, selects, textareas, editores inline e multiselects devem parecer parte do
mesmo sistema de superfícies, inclusive em dark mode:

- controles canônicos usam `.pc-input` com `--surface-overlay`, `--border`,
  `--shadow-sm`, `min-height: 40px` e `font: inherit`;
- textareas canônicos usam `textarea.pc-input`, `min-height: 96px`, `line-height:
  1.55` e resize vertical;
- selects devem usar `SelectField`/`.pc-select-wrapper`/`.pc-select` quando
  possível, com `appearance: none` e ícone `chevron-down`;
- `pc-tag-multiselect` compartilha a mesma superfície, borda, sombra e anel de
  foco dos campos base;
- campos inline (`InlineTextField`, `InlineTextareaField`) mantêm fundo
  transparente em repouso, mas herdam fonte, borda e raio do sistema para hover e
  foco previsíveis;
- mudanças nesses controles devem manter
  `TestDesignSystemCSSDefinesCanonicalFormControls` verde.

### Badge/status identity

Badges, chips e indicadores de status aparecem em listas, detalhes e configurações. Para manter uma linguagem única:

- `.badge`, `.pc-badge` e `.pc-chip` compartilham `--badge-radius` e `--badge-border`;
- variantes `good`, `warn`, `bad` e `info` usam as mesmas cores semânticas já existentes;
- dots de `.status` usam `--status-dot-size` e um halo sutil derivado de `currentColor`;
- badges devem ser compactos, em uppercase quando forem metadados/status, e nunca criar novas cores locais;
- mudanças nesse grupo devem manter `TestDesignSystemCSSDefinesConsistentBadgesAndStatus` verde.

## Componentes Templ canônicos

Definidos principalmente em `internal/adapters/web/ui.templ`.

| Componente | Uso |
| --- | --- |
| `Card`, `CardEdit` | Containers de conteúdo. |
| `SectionHeader` | Cabeçalho de seção com descrição e actions. |
| `Button`, `ButtonLink`, `IconButton` | Ações primárias, secundárias, ghost e danger. |
| `OperationalLink` | Link discreto para runbooks em superfícies críticas sem competir com ações primárias. |
| `InlineTextField`, `InlineTextareaField` | Edição inline com HTMX. |
| `FormField`, `FieldLabel`, `FieldMessage` | Estrutura acessível para campos. |
| `TextField`, `TextareaField`, `SelectField` | Inputs base. |
| `TagMultiSelectField` | Editor de tags com chips e hidden inputs. |
| `EnvEditorField` | Editor de variáveis de ambiente com máscara, validação e reveal. |
| `Chip`, `Badge`, `StatusBadge` | Metadados, tags e status. |
| `PropertyList`, `PropertyItem`, `PropertyItemHTML` | Detail rails e metadados estruturados. |
| `EmptyState` | Estados vazios com ícone, título, mensagem e actions. |
| `LoadingState` | Estado de carregamento com spinner e mensagem opcional (para polling e live regions). |
| `ErrorState` | Estado de erro estruturado com ícone, mensagem e bloco `<pre>` de detalhes técnicos. |
| `OverviewGrid`, `OverviewCard` | Cards de métricas/overview em Dashboard, Tasks, Runs e Activity, usando classes `pc-overview-*` e tons canônicos. |
| `Tabs`, `TabLink` | Navegação local por abas. |
| `EntityRow` | Lista compacta de entidades navegáveis. |
| `CommandPalette` | Modal global de busca/comandos. |

Antes de criar um novo helper visual, procure se um destes já resolve o caso.

## Ícones

Ícones são SVG inline via `Icon(name, size)` em `icons.templ`.

Regras:

- adicionar somente ícones usados pela UI;
- não introduzir icon font;
- não depender de CDN;
- usar `IconButton` com `aria-label` para ações icon-only;
- manter nomes semânticos e consistentes.

Ícones vistos em navegação/componentes incluem, entre outros:

- `paperclip`
- `layout-dashboard`
- `check-square`
- `play-circle`
- `activity`
- `folder`
- `bot`
- `sparkles`
- `settings`
- `check`
- `x`
- `plus`
- `trash`
- `eye`
- `chevron-down`
- `chevron-right`
- `cpu`
- `book`
- `zap`

## Layout patterns

### Page header

Toda página principal deve usar o header do `PageShell`:

- eyebrow com área ativa;
- ícone derivado de `pageIcon(active)`;
- título;
- subtítulo;
- action slot quando necessário.

Não recrie headers locais sem necessidade.

### Responsive shell

O shell mantém a navegação lateral fixa em desktop. Em telas até `980px`, a sidebar vira uma barra superior `sticky`, compacta e com navegação horizontal rolável. Esse padrão reduz altura ocupada no mobile, preserva acesso rápido às áreas principais e evita overflow horizontal da página.

Regras:

- a sidebar mobile deve permanecer `position: sticky; top: 0` para manter navegação acessível;
- links de navegação devem usar `flex: 0 0 auto` e `white-space: nowrap` dentro de um nav com `overflow-x: auto`;
- a área principal deve reduzir padding em telas pequenas para aproveitar largura útil;
- o brand pode ocultar subtítulo secundário no mobile, mas deve manter o nome PicoClip e ícone;
- mudanças no shell responsivo devem manter o teste `TestResponsiveShellCSSKeepsMobileNavigationCompact` verde.

### Detail pages

Use `detail-grid` para páginas de detalhe:

- coluna principal: conteúdo, conversas, runs, activity, formulários principais;
- rail lateral: propriedades, actions, delegation, danger zone.

Exemplos de detalhe:

- task detail;
- run detail;
- agent detail;
- project detail;
- skill detail;
- webhook detail.

### Dashboards e listas

Use:

- `metrics`/`metrics-dashboard` para métricas;
- `OverviewGrid`/`OverviewCard` para resumos canônicos de Dashboard, Tasks, Runs e Activity;
- cards de overview devem manter fluxo vertical legível — label, métrica e legenda empilhados, sem grid lado-a-lado que comprima números grandes ou faça texto competir com decoração;
- cards com status visual para atenção operacional;
- `table-wrapper` para tabelas;
- tabs para filtros de status;
- empty states quando listas estiverem vazias.

Em telas até `768px`, grids de overview canônicos devem empilhar em uma coluna (`.pc-overview-grid`) e headers de painéis operacionais devem virar coluna com conteúdo ocupando a largura disponível (`.dashboard-panel-header`, `.tasks-panel-header`, `.runs-panel-header`, `.activity-panel-header`). Pills/filtros compactos como `.runs-filter-pill` e `.activity-live-pill` permanecem alinhados ao início para não parecerem botões full-width. Esse contrato evita overflow e mantém ações alcançáveis no mobile; mudanças devem manter `TestMobileDashboardCSSStacksCanonicalOverviewAndPanelHeaders` e `TestMobilePanelHeadersStackAcrossOperationalPages` verdes.

## HTMX patterns

### Forms

Para ações que modificam estado:

- use `hx-post` ou método apropriado;
- adicione mensagens de sucesso/erro via toast quando aplicável;
- preserve comportamento de form sempre que possível;
- evite swaps grandes quando uma resposta pequena basta.

O shell escuta eventos HTMX para:

- colocar forms em loading;
- desabilitar botão submit;
- restaurar botão após request;
- mostrar toast para requests não-GET;
- ler `HX-Trigger` com evento `picoclip-toast`.

### Full page refresh vs partials

Aceitável para ações simples:

```html
hx-target="body" hx-swap="outerHTML"
```

Preferido para regiões live orientadas a eventos:

```html
<div id="task-live"
  hx-get="/partials/tasks/{id}"
  hx-swap="innerHTML"
  data-task-id="{id}"
  data-task-live-url="/partials/tasks/{id}">
</div>
<script>
  const source = new EventSource('/sse/tasks/' + taskID)
  source.onmessage = () => htmx.ajax('GET', '/partials/tasks/' + taskID, '#task-live')
</script>
```

Polling pequeno ainda é aceitável como fallback ou para regiões sem eventos persistidos, mas a preferência para áreas colaborativas/live é SSE filtrado + partial pequeno. Nunca faça polling frequente de `<body>` inteiro.

### Partials

Use `/partials/...` para regiões que precisam atualizar sem destruir estado local da página.

Padrões atuais:

- task live/detail regions;
- run live output/status regions.

Ao criar partial:

1. registre rota em `server.go`;
2. mantenha payload pequeno;
3. garanta que o fragmento é válido isoladamente;
4. não coloque modais ou forms críticos dentro de containers pollados;
5. documente rota em [API Reference](API_REFERENCE.md) se for relevante para agentes/manutenção.

## SSE e live observability

Activity usa SSE em `/sse/activity`. Detalhe de task usa SSE filtrado em `/sse/tasks/{id}` para acionar refresh do fragmento `/partials/tasks/{id}` somente quando um evento da própria task chega. Detalhe de run usa `/sse/runs/{id}/logs` para anexar output incremental e acionar refresh do fragmento `/partials/runs/{id}` quando a run termina, sem polling periódico.

Padrão esperado:

- página inicial server-rendered;
- `EventSource` como progressive enhancement;
- stream SSE filtra no servidor quando a página tem escopo claro (`task_id`, `run_id`);
- evento SSE não precisa carregar HTML: ele pode apenas acionar refresh de um partial pequeno;
- fallback continua útil sem SSE: em erro de conexão, a região live pode degradar para refresh parcial espaçado, sem voltar a polling agressivo;
- conexões `EventSource` devem ser fechadas em navegação/saída da página para evitar streams órfãos;
- eventos importantes aparecem em Activity e/ou run/task detail.

Eventos de robustez, retry, driver e budget devem ser visíveis o suficiente para investigação operacional. Veja [Robustness](ROBUSTNESS.md) e [Operations Runbook](OPERATIONS.md).

## Custom elements leves

O shell registra custom elements pequenos em `layout.templ`.

### `pc-tag-multiselect`

Usado por `TagMultiSelectField`.

Comportamento:

- filtra tags disponíveis localmente;
- cria tag nova inline;
- renderiza chips;
- submete hidden inputs repetidos;
- suporta ArrowUp, ArrowDown, Enter, Escape e Backspace.

Handlers devem ler valores repetidos do campo antes de fallback legado.

### `pc-env-editor`

Usado por `EnvEditorField`.

Comportamento:

- adiciona/remove linhas;
- normaliza chaves para uppercase com `_`;
- remove caracteres inválidos;
- impede chaves começando por dígitos;
- destaca duplicatas;
- mascara valores por padrão;
- permite reveal manual.

Regra: não exibir valores sensíveis em docs, logs ou screenshots. Na UI, valores sensíveis devem ser revelados apenas por ação explícita.

## Command palette

A command palette global é aberta por `Ctrl+K` / `Cmd+K`.

Ela:

- pesquisa via `/api/search?q=...`;
- renderiza resultados client-side;
- permite navegação por setas;
- usa Enter para abrir item selecionado;
- fecha com Escape ou clique no backdrop.

Ao adicionar novo tipo pesquisável:

1. atualize `/api/search`;
2. ajuste rendering se necessário;
3. documente no [API Reference](API_REFERENCE.md);
4. adicione teste/smoke se o fluxo for importante.

## Modais

Modais usam atributos de dados:

- `data-open-modal`
- `data-close-modal`
- `data-modal`

Regras:

- manter modais fora de fragments com polling;
- Escape fecha todos;
- clique no backdrop fecha;
- abrir modal foca primeiro input/textarea/select/button;
- ações destrutivas devem usar estilo danger e confirmação explícita;
- danger zones devem ficar visualmente separadas de actions normais.

## Padrões por página

### Dashboard

Dashboard deve ser centro operacional:

- top row com open work, running runs, blocked work, failures;
- coluna principal com trabalho que precisa atenção;
- coluna lateral com saúde do sistema e activity recente;
- falhas críticas não devem ficar escondidas abaixo da dobra.

### Tasks

Tasks devem deixar claro:

- status;
- agent/workspace;
- modo `once` vs `continuous`;
- tentativas;
- lock/run ativo;
- subtasks/delegation quando houver;
- comments separados de system activity.

### Runs

Runs são registros de observabilidade:

- `/runs` lista execuções com filtros/status tabs;
- `/runs/{id}` mostra output, erro, input context e metadata;
- running runs usam partial polling focado;
- timeout/retry deve ser legível sem abrir o banco.

### Activity

Activity deve transformar eventos técnicos em mensagens humanas.

Eventos importantes:

- `run.timeout`;
- `run.recovered`;
- `retry.scheduled`;
- `budget.blocked`;
- `driver.missing`.

### Settings

Settings contém configuração operacional e danger zone. Configuração de runtimes inclui o Runtime Provider Quick Setup. Runtimes configurados ocupam a largura completa da grade para não comprimir configuração e diagnóstico. Quick Setup usa seções sequenciais para endpoint e credenciais/modelo, mantém ações separadas do conteúdo e empilha campos e botões no mobile. Formulários devem manter configurações avançadas recolhidas, usando `<details>` sem estado `open`, e tratar `409 Conflict` ou revisões de concorrência recarregando a página via `HX-Refresh`. Formulários não devem emitir toasts conflitantes entre atributos HTML e Server Headers.

Regras:

- separar adapters/runtimes de danger zone;
- mostrar diagnósticos de forma copiável/acionável;
- operações destrutivas precisam confirmação;
- backup/restore deve apontar para [Storage Architecture](STORAGE.md) e [Operations Runbook](OPERATIONS.md).

## Acessibilidade e UX checklist

Antes de finalizar UI:

- [ ] A página tem título/subtítulo claros via `PageShell`?
- [ ] Botões icon-only têm `aria-label`?
- [ ] Campos têm label ou contexto explícito?
- [ ] Estados empty/error/loading são legíveis?
- [ ] Toast não é a única fonte de informação crítica?
- [ ] Modais focam um controle ao abrir e fecham com Escape?
- [ ] Ações destrutivas são visualmente separadas?
- [ ] Polling não destrói input em edição?
- [ ] Dark theme continua legível?
- [ ] Console do browser fica limpo?

## History cleanup actions

Runs and Activity pages expose compact danger bars for clearing history. These bars must explain exactly what is preserved, use `pc-btn-danger`, and require browser confirmation before posting. Task rows expose a direct `Delete task` action beside `Open task`; the row remains keyboard/link accessible while destructive controls stay separate from the row link.

## E2E e validação de UI

Validação mínima ao alterar UI/Templ/CSS:

```sh
make templ-generate
go test ./internal/adapters/web -count=1
```

Validação recomendada:

```sh
make test-e2e
```

Validação completa:

```sh
make check
```

Se estiver em Alpine e o browser do Playwright falhar, use a orientação em [Development Guide](DEVELOPMENT.md).

## Checklist para nova página

1. Defina rota em `server.go`.
2. Crie/atualize ViewModel no handler.
3. Renderize com `PageShell`.
4. Use componentes de `ui.templ`.
5. Use ícone existente ou adicione SVG inline necessário.
6. Adicione nav item se for página top-level.
7. Crie partials apenas para regiões live.
8. Adicione teste web ou E2E smoke proporcional.
9. Atualize [Project Map](PROJECT_MAP.md).
10. Atualize [API Reference](API_REFERENCE.md) se houver rota/API relevante.
11. Atualize este documento se introduzir padrão novo.

## Checklist para novo componente

1. Verifique se `ui.templ` já resolve.
2. Nomeie com prefixo/convenção `pc-*` quando CSS for componente.
3. Use tokens existentes.
4. Garanta variante de dark theme se necessário.
5. Adicione estado disabled/loading/error quando aplicável.
6. Inclua acessibilidade mínima (`aria-*`, labels, foco).
7. Evite JavaScript se HTML/CSS resolver.
8. Se usar JS, registre de forma idempotente e compatível com HTMX swaps.
9. Documente aqui se for reutilizável.

## Antipadrões

Evite:

- duplicar markup complexo em múltiplas páginas;
- adicionar framework SPA ao core;
- carregar assets externos/CDNs;
- criar CSS page-specific para algo reutilizável;
- polling de página inteira;
- modais dentro de containers pollados;
- esconder failures críticos em texto pequeno;
- usar apenas cor para distinguir status;
- exibir valores sensíveis de env vars por padrão;
- editar manualmente arquivos `*_templ.go` gerados.

## Manutenção deste documento

Atualize este documento quando:

- criar componente Templ reutilizável;
- alterar shell, navegação, tema, command palette ou modais;
- introduzir novo padrão HTMX/SSE;
- mudar estrutura de dashboard, task detail, run detail ou settings;
- adicionar regras de acessibilidade/UX;
- alterar workflow Templ ou validação de UI.
