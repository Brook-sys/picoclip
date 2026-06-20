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

Ele já inclui uma UI web funcional, gerenciamento de ciclo de vida de tasks, agentes, skills, projetos/workspaces, uma API administrativa local e uma Agent API para fluxos conduzidos por agentes.

Mesmo assim, ainda é cedo. Alguns comportamentos continuam sendo refinados, e partes do sistema podem mudar bastante com o tempo.

## Roadmap

Existe um roadmap ativo, e mais recursos serão adicionados gradualmente conforme o projeto amadurece.

Veja:

- [Roadmap](docs/ROADMAP.md)
- [Estado Atual](docs/CURRENT_STATE.md)
- [Arquitetura de Storage](docs/STORAGE.md)
- [Guia de Desenvolvimento](docs/DEVELOPMENT.md)

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
