# PicoClip — Ciclo Autônomo de Melhorias

Este documento define como o PicoClip usa o Hermes Kanban e o cron autônomo para descobrir, registrar, priorizar e executar melhorias contínuas.

Ele complementa:

- [Project Map](PROJECT_MAP.md)
- [Documentation Policy](DOCUMENTATION_POLICY.md)
- [Development Guide](DEVELOPMENT.md)
- [Current State](CURRENT_STATE.md)
- [Roadmap](ROADMAP.md)

## Objetivo

O ciclo autônomo deve permitir que agentes do Hermes melhorem o PicoClip sem depender de prompts manuais a cada rodada, mas sem perder segurança operacional.

O ciclo deve:

1. consultar o board Hermes Kanban do PicoClip;
2. descobrir novas demandas reais quando houver lacunas;
3. criar cards pequenos, objetivos e verificáveis;
4. executar no máximo uma melhoria por rodada;
5. validar, commitar e pushar somente quando tudo passar;
6. atualizar o Kanban com início, resultado, bloqueios e follow-ups;
7. evitar duplicação entre execuções manuais e o cron de 30 minutos;
8. manter o PicoClip rodando localmente em `8088` quando a rodada terminar.

## Fonte de verdade

A fonte operacional das melhorias é o board Hermes Kanban:

```text
board: picoclip
tenant: picoclip
```

Use:

```sh
hermes kanban boards switch picoclip
hermes kanban list --tenant picoclip --sort priority-desc
hermes kanban stats
```

Roadmap e documentação continuam sendo a fonte estratégica e arquitetural. O Kanban organiza a fila executável.

## Escopo do ciclo autônomo

Cada rodada deve ter escopo pequeno e vertical.

Permitido:

- criar até 3 cards novos quando encontrar lacunas reais;
- executar exatamente 1 card `ready` por rodada;
- atualizar documentação proporcional;
- adicionar ou ajustar testes;
- commitar e pushar quando a validação passar;
- bloquear um card quando houver impedimento real.

Não permitido:

- executar redesign amplo em uma única rodada;
- misturar várias demandas independentes no mesmo commit;
- commitar workspace sujo de outro trabalho;
- adicionar `graphify-out/`, bancos locais, screenshots, logs ou artefatos temporários;
- documentar comportamento futuro como se já estivesse implementado;
- insistir em correções arriscadas após falhas repetidas de validação;
- criar novos cron jobs a partir de uma execução cron.

## Formato obrigatório de cards

Cards novos devem ser específicos, pequenos e verificáveis.

Um bom card inclui:

```text
Contexto:
Por que esta melhoria importa e qual lacuna real foi observada.

Arquivos prováveis:
- caminho/arquivo.go
- docs/AREA.md

Critério de aceite:
- comportamento ou documentação esperada;
- limite de escopo;
- estado vazio/erro quando aplicável.

Validação esperada:
- teste focado;
- make templ-generate quando templates mudarem;
- make check quando a mudança for relevante.

Nota de documentação:
Quais documentos devem mudar ou por que não precisam mudar.
```

Evite cards vagos como:

```text
melhorar UI
melhorar backend
refatorar API
```

Prefira cards executáveis:

```text
Padronizar estados empty/loading/error da UI
Adicionar filtros rápidos de Activity por tipo e entidade
Classificar falhas retryable vs non-retryable no runner/reconciler
```

Use `--idempotency-key` estável para evitar duplicatas:

```sh
hermes kanban create "Título específico" \
  --tenant picoclip \
  --priority 80 \
  --idempotency-key picoclip-area-feature-v1 \
  --body "..."
```

## Política de priorização

Ao escolher o próximo card, avalie nesta ordem:

1. **Segurança do workspace**
   - Se `git status --short` mostra alterações rastreadas não relacionadas, não execute nova mudança.
   - Preserve arquivos não rastreados conhecidos, especialmente `graphify-out/`.

2. **Bloqueios e dependências**
   - Não execute card que depende de outro ainda não concluído.
   - Se a dependência não está explícita, comente ou bloqueie o card.

3. **Prioridade Kanban**
   - Prefira maior `priority`.
   - Em empate, prefira cards menores e com validação clara.

4. **Risco e tamanho**
   - Prefira mudanças pequenas, reversíveis e com teste direto.
   - Quebre cards grandes em follow-ups antes de executar.

5. **Valor operacional**
   - Priorize melhorias que aumentem percepção de agentes, robustez, validação, UI operacional ou documentação de fluxo.

6. **Coerência com o estado atual**
   - Leia docs e código antes de executar.
   - Não presuma que o roadmap está 100% atualizado; confirme no código.

## Limites por rodada

Cada execução autônoma deve respeitar estes limites:

| Limite | Regra |
| --- | --- |
| Cards novos | Até 3 por rodada, salvo descoberta crítica. |
| Cards executados | Exatamente 1 card por rodada. |
| Commits | Normalmente 1 commit por card. |
| Escopo | Uma fatia vertical verificável. |
| Validação | Teste focado + validação canônica proporcional. |
| Retries de correção | Depois de cerca de 3 tentativas no mesmo arquivo/erro, bloquear ou pedir revisão. |

## Estados e comentários no Kanban

Antes de editar:

```sh
hermes kanban show <id>
hermes kanban comment <id> "Iniciando: plano curto, arquivos esperados e validação prevista."
```

Durante a execução:

- comente descobertas relevantes;
- crie follow-ups quando surgirem lacunas fora do escopo;
- não esconda falhas de validação.

Ao concluir:

```sh
hermes kanban comment <id> "Concluído: resumo, validações, commit e observações."
hermes kanban complete <id>
```

Ao bloquear:

```sh
hermes kanban block <id> "Bloqueador concreto, comando que falhou e próximo passo sugerido."
```

## Quando bloquear em vez de insistir

Bloqueie ou pare a rodada quando houver:

- conflito Git ou workspace sujo por outro trabalho;
- falha de validação que exige mudança maior que o card;
- dependência externa indisponível;
- comando ou API necessária ausente;
- dúvida de produto que muda o critério de aceite;
- risco de expor segredo ou tocar arquivo sensível;
- mudança que exigiria redesign amplo ou reestruturação de arquitetura.

O comentário de bloqueio deve incluir:

- comando executado;
- erro observado;
- arquivos afetados;
- próximo passo recomendado.

## Coordenação com cron de 30 minutos

Existe um job Hermes para o ciclo autônomo:

```text
PicoClip autonomous planner and improvement cycle
```

Agenda desejada:

```text
every 30m
```

Em sessões manuais, pause temporariamente esse job antes de editar o repo para evitar concorrência:

```sh
hermes cron pause <job-id>
```

Ao final, reative:

```sh
hermes cron resume <job-id>
```

Se a execução estiver rodando em ambiente TUI com `deliver=local`, a saída fica salva no histórico do job e pode não aparecer como mensagem ao vivo.

## Validação obrigatória

Antes de commit:

```sh
git status --short
git diff --check
git diff --stat
```

Para docs:

```sh
python3 - <<'PY'
from pathlib import Path
import re
files = [Path('README.md'), Path('README.pt-BR.md'), Path('AGENTS.md'), *Path('docs').glob('*.md')]
missing = []
for f in files:
    if not f.exists():
        continue
    text = f.read_text(errors='ignore')
    for m in re.finditer(r'\]\(([^)]+)\)', text):
        link = m.group(1).strip()
        if link.startswith(('http://', 'https://', 'mailto:')):
            continue
        path = link.split('#', 1)[0]
        if not path:
            continue
        target = (f.parent / path).resolve()
        if not target.exists():
            missing.append((str(f), link, str(target)))
print('missing=', missing)
raise SystemExit(1 if missing else 0)
PY
```

Para mudanças de código, UI, templates, runtime, API ou docs integradas ao fluxo de desenvolvimento:

```sh
make check
```

## Revisão pré-commit

Antes de commitar:

1. revisar `git diff`;
2. rodar `git diff --check`;
3. escanear diff para termos sensíveis como `secret`, `password`, `api_key`, `bearer`, `credential`;
4. garantir que `graphify-out/` e artefatos locais não serão adicionados;
5. confirmar que a documentação não apresenta planos futuros como estado atual.

## Resposta final esperada

Toda rodada deve terminar com resumo em português contendo:

- card trabalhado;
- cards novos criados;
- alterações feitas;
- validações reais;
- commit/push;
- status do servidor `8088` local e Tailscale;
- estado do Kanban;
- passos restantes;
- melhorias ainda recomendadas;
- correções pendentes.

## Relação com documentação do produto

- Use [Roadmap](ROADMAP.md) para buscar lacunas estratégicas.
- Use [Current State](CURRENT_STATE.md) para confirmar estado real.
- Use [Documentation Policy](DOCUMENTATION_POLICY.md) para decidir quais docs atualizar.
- Use [Development Guide](DEVELOPMENT.md) para comandos e validação.
- Use [Design System](DESIGN.md), [API Reference](API_REFERENCE.md), [Robustness](ROBUSTNESS.md), [Storage Architecture](STORAGE.md) e [Operations Runbook](OPERATIONS.md) conforme a área do card.
