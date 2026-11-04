# Cliente: plugin no Microbot (fork do RuneLite)

Data: 2026-11-03

## Por que isso muda o plano

O dono já desenvolve plugins para o Microbot (https://github.com/chsami/Microbot), um fork do RuneLite
(licença BSD-2, mesma do RuneLite) com um plugin sempre ativo para scripts de automação. Para o planner
isso resolve o maior gargalo identificado nos docs 03 e 05: quests, diaries e unlocks são o sinal
dominante (159 dos 209 steps auto-rastreáveis) e só saem do cliente. Com o Microbot:

- Não há revisão do Plugin Hub, nem limite do que o plugin pode ler. Sideload é o fluxo normal.
- A API do RuneLite continua disponível inteira (`Client`, `Quest.getState`, varbits, `ItemContainer`
  do banco e inventário, `Skill` xp), então o cliente implementa o contrato v1 do doc 04, seção 6.4.
- Por cima, o Microbot oferece caches consultáveis (`Microbot.getRs2XxxCache()` para NPCs, objetos,
  inventário, banco, varbits, players, widgets) e utilitários `Rs2Bank`, `Rs2Inventory`, `Rs2Player`,
  `Rs2Walker`, `Rs2Dialogue`, que já cuidam de thread do cliente e retries. Documentação em
  `runelite-client/src/main/java/net/runelite/client/plugins/microbot/AGENTS.md` e `api/QUERYABLE_API.md`.

Consequência: o plugin próprio sai de "v1 ou v2" para o MVP, e o importador WikiSync vira um
fallback para quem não usa o Microbot, ou pode nem existir.

## Escopo do plugin (somente leitura)

O planner só precisa de leitura. Nada de automação no plugin do planner; isso evita misturar o risco
de ban dos scripts com a conta ironman do dono. Separar em plugin próprio, com nome próprio, dentro
do repositório de plugins que ele já mantém.

| Dado | Como ler no cliente | Quando enviar |
|---|---|---|
| Skills (level real, xp, boosted) | `client.getRealSkillLevel`, `getSkillExperience`, evento `StatChanged` | no evento, com debounce de alguns segundos |
| Quests (NOT_STARTED, IN_PROGRESS, FINISHED) | `Quest.values()` + `quest.getState(client)` na thread do cliente, enviados na faceta `data.quest` do contrato v1 (doc 04, seção 6.4). **Ainda não implementado**: entra na versão 1.1.0 (doc 09, item 7) | todos no `full`; em `delta` só os que mudaram |
| Diaries por tier | varbits `DIARY_<ÁREA>_<TIER>_COMPLETED` de `content/reference/varbits.json`, pedidos pelo manifest (hoje o manifest sai vazio porque o conteúdo não tem regras `varbit`) | em `VarbitChanged` |
| Combat achievements | varbits/cs2 do manifest | em `VarbitChanged` |
| Collection log | `collectionLogSlots` via enums/structs do chisel, como o WikiSync faz | login e ao abrir o log |
| Banco, inventário, equipamento | `ItemContainerChanged` para `live`; cache `Rs2Bank`/`Rs2Inventory` apenas como `restored` | ao fechar o banco, ao logar; opt-in separado por container |
| Posição, mundo, membro | `client.getLocalPlayer().getWorldLocation()`, `getWorld()` | a cada N segundos, opt-in; útil para "você está em Ardougne, próximo step aqui" |
| Quest Helper ativo | não necessário | |

Protocolo: idêntico ao doc 04, seção 6.4. `GET /api/ingest/manifest` devolve
`contract_version: 1`, `manifest_version`, varbits/varps/coleções, `token_scopes`, permissões de
itens e `resync`. `POST /api/ingest` envia `client_seq` monotônico e `kind: full|delta`; a resposta
repete `accepted_seq`, a versão e se exige `resync`. O token `ipl_` fica por conta na config.

Os toggles `sendBank`, `sendInventory` e `sendEquipment` começam `false` e só coletam se o manifest
também anunciar o scope correspondente (`items:bank`, `items:inventory`, `items:equipment`). Um
`full` sempre descreve os três containers: sem permissão/toggle/leitura usa `state: unavailable`,
nunca `{}`. Um container restaurado de cache usa `state: restored`, conserva o `observed_at` da
captura original, leva `revision` e o mapa inteiro, mas **não** leva `live_epoch`. Assim ele pode
servir de espelho sem fingir que houve evento do banco no login atual.

Somente uma leitura/evento na thread do cliente produz `state: live`. No login o plugin cria um
`live_epoch` opaco; a `revision` começa em zero e cresce por container dentro desse epoch. Em `full`,
`quantities` é o retrato inteiro e não aceita zero. Em `delta`, containers inalterados são omitidos e
o mapa de um container `live` usa total novo por item (`0` remove). Os mapas no DTO Java devem ser
`Map<String, Long>` para serializar como `{"4151": 1}`, sem adaptador de chaves do Gson.

## Esqueleto sugerido no repositório de plugins do Microbot

```
plugins/ironmanplanner/
  IronmanPlannerPlugin.java     @PluginDescriptor(name = "Ironman Planner", tags = {"ironman","tracker"})
  IronmanPlannerConfig.java     serverUrl, ingestToken, sendBank/sendInventory/sendEquipment=false
  PlannerClient.java            OkHttp (já no RuneLite), manifest GET, submit POST com Gson
  PlayerStateCollector.java     lê estado; rastreia observed_at/revision/live_epoch; calcula delta
  PlannerPanel.java (opcional)  painel lateral com "próximo step" vindo de GET /api/now
```

Regras do AGENTS.md do Microbot que importam aqui: tudo que lê o `Client` roda na thread do cliente
(`Microbot.getClientThread().runOnClientThreadOptional` ou `invokeLater`), rede nunca na thread do
cliente, e scripts em `ScheduledExecutorService` próprio. O DTO de rede copia os mapas ainda na
thread do cliente; o envio assíncrono não segura referências mutáveis do cache.

## Bônus que só o Microbot permite

- Painel in-client com o próximo step do guia e botão "feito", conversando com
  `PATCH /api/guides/{guide}/steps/{id}/mark` (doc 04, seção 2.5).
- Overlay de "itens que o guia manda guardar" ao abrir o banco, comparando com `data/guide` `items`.
- Detecção de contexto: se o jogador está perto do NPC ou região de um step pendente, destacar na UI web.
- Em fase bem posterior, opcional e separado: scripts de rotina (ex.: ir ao banco e pegar o inventário
  do próximo step) usando `Rs2Walker` e `Rs2Bank`. Fora do escopo do planner.

## Impacto nos outros docs

- Doc 03: F8 (plugin próprio) passou para "M MVP" e F7 (WikiSync) para "C v2" (aplicado em 2026-11-04).
- Doc 04, seção 9: o ingest e o plugin entraram antes do progresso persistido.
- Doc 01, decisão 2 (WikiSync no MVP): resolvida pelo plugin no Microbot.

## Adendo de 2026-11-04: controle do cliente

O dono decidiu controlar o Microbot pelo webapp (ver o jogo ao vivo, iniciar e parar plugins,
rotinas). Isso vive num módulo de controle separado, desligado por padrão, descrito no doc 08. O
plugin de tracking deste doc continua somente leitura.

## Adendo de 2026-11-04: plugin implementado

O plugin existe em `FelipeMalacarne/microbot-plugins` (repositório privado), pacote
`net.runelite.client.plugins.microbot.fm.ironmanplanner`, versão 1.0.0. O esqueleto acima virou estes
arquivos:

- `IronmanPlannerPlugin` e `IronmanPlannerConfig`: URL do servidor, token e toggles de banco, inventário
  e equipamento, desligados por padrão.
- `StateCollector`: lê o estado na thread do cliente.
- `ContainerTracker` e `BankFreshness`: revisão e `live_epoch` por container.
- `PushBuilder`: monta `full` e `delta` e decide quando um `full` é obrigatório.
- `SyncEngine`: sequência, ACK, retry e backoff.
- `PlannerHttpClient` e `PlannerStatusOverlay`.

Comportamento implementado (contrato v1, doc 04, seção 6.4):

- O banco fica `restored` até um evento de container BANK nesta sessão de login aplicar um `live_epoch`.
- Qualquer troca de estado ou de epoch força `full`.
- Deltas só saem depois de um ACK, com o mesmo `client_seq` no retry após 3 s de timeout e backoff
  quadrático.
- Um 409 faz o plugin renumerar e mandar `full`.
- O token e o username nunca aparecem em log, e o plugin não executa nenhuma ação no jogo.
- O log de coleção (opcional no contrato) ainda não é enviado.
- **Quests e diaries ainda não chegam ao servidor**: o plugin lê só os varbits do manifest (vazio, porque
  o conteúdo não tem regras `varbit`) e as skills; não há leitura de `Quest.getState` nem faceta `quest`
  no contrato. É o item 7 da sequência do doc 09 (versão 1.1.0 do plugin, junto com o servidor).

Verificação: 36 testes JUnit (diff, baseline, frescor, ACK/retry, backoff) e o jar compilado contra o
cliente pinado. **Ainda não rodou no jogo** (`mb-test` pendente). O painel lateral com o "próximo passo"
não foi feito.
