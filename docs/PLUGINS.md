# Plugins via gRPC

O PicoClip possui um **contrato inicial** para plugins externos via gRPC. O objetivo é permitir que ferramentas escritas em Go, Python, Rust, Node.js ou outra linguagem compatível com gRPC sejam integradas sem ampliar o core com dependências específicas.

## Estado atual

Entregue:

- contrato Protobuf versionado em `api/plugin/v1/plugin.proto`;
- stubs Go gerados para cliente e servidor;
- RPC `Discovery` para descrever ferramentas;
- RPC `Execute` para executar uma ferramenta;
- configuração de geração em `buf.yaml` e `buf.gen.yaml`.

Ainda não entregue:

- registry/configuração de plugins;
- processo host que conecte o contrato ao runner e às tools dos agentes;
- gerenciamento de lifecycle, restart e health;
- UI em Settings;
- autenticação, autorização e sandbox do processo externo;
- SDKs e exemplos executáveis por linguagem.

Portanto, a presença do contrato **não torna plugins executáveis pelo PicoClip ainda**. Ela estabelece a fronteira estável sobre a qual o host será implementado.

## Contrato

O serviço está no pacote `picoclip.plugin.v1`:

```proto
service PluginService {
  rpc Discovery(DiscoveryRequest) returns (DiscoveryResponse);
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}
```

`Discovery` retorna as ferramentas expostas pelo plugin, incluindo nome, descrição e JSON Schema de entrada. `Execute` recebe o nome da ferramenta e argumentos JSON e retorna resultado, erro estruturado e metadata de execução.

Consulte o arquivo canônico:

```text
api/plugin/v1/plugin.proto
```

## Gerar os stubs

Com Buf instalado:

```sh
buf generate
```

O CI e os testes devem tratar os arquivos gerados como parte versionada do contrato. Mudanças incompatíveis exigem uma nova versão do pacote Protobuf, em vez de alterar silenciosamente `v1`.

## Próximas etapas recomendadas

1. implementar um `PluginRegistry` no core;
2. criar um host gRPC com conexão e health explícitos;
3. adaptar ferramentas descobertas para a porta de execução usada pelos agentes;
4. definir limites, permissões, timeouts e cancelamento;
5. adicionar plugin de exemplo e testes de integração;
6. somente então documentar configuração operacional e UI como funcionalidades disponíveis.

O design deve continuar local-first: plugins são opcionais e a ausência deles não pode impedir a inicialização ou o uso normal do PicoClip.
