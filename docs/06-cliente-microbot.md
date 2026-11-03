# Cliente: plugin no Microbot (fork do RuneLite)

Data: 2026-11-03

## Por que isso muda o plano

O dono já desenvolve plugins para o Microbot (https://github.com/chsami/Microbot), um fork do RuneLite
(licença BSD-2, mesma do RuneLite) com um plugin sempre ativo para scripts de automação. Para o planner
isso resolve o maior gargalo identificado nos docs 03 e 05: quests, diaries e unlocks são o sinal
dominante (159 dos 209 steps auto-rastreáveis) e só saem do cliente. Com o Microbot:

- Não há revisão do Plugin Hub, nem limite do que o plugin pode ler. Sideload é o fluxo normal.
- A API do RuneLite continua disponível inteira (`Client`, `Quest.getState`, varbits, `ItemContainer`
  do banco e inventário, `Skill` xp), então o contrato de ingestão do doc 04, seção 6.4, vale sem mudança.
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
| Quests (NOT_STARTED, IN_PROGRESS, FINISHED) | `Quest.values()` + `quest.getState(client)` na thread do cliente | login e a cada `VarbitChanged` que bata no manifest |
| Diaries por task e por tier | varbits do manifest (o servidor manda a lista, como no WikiSync) | em `VarbitChanged` |
| Combat achievements | varbits/cs2 do manifest | em `VarbitChanged` |
| Collection log | `collectionLogSlots` via enums/structs do chisel, como o WikiSync faz | login e ao abrir o log |
| Banco, inventário, equipamento | `ItemContainerChanged` ou cache `Rs2Bank`/`Rs2Inventory` | ao fechar o banco, ao logar; opt-in por toggle |
| Posição, mundo, membro | `client.getLocalPlayer().getWorldLocation()`, `getWorld()` | a cada N segundos, opt-in; útil para "você está em Ardougne, próximo step aqui" |
| Quest Helper ativo | não necessário | |

Protocolo: idêntico ao doc 04, seção 6.4. `GET /api/ingest/manifest` (lista de varbits, varps,
coleções e versão), `POST /api/ingest` com deltas, `client_seq`, `resync` quando o servidor pede
estado completo, token `ipl_` por conta na config do plugin.

## Esqueleto sugerido no repositório de plugins do Microbot

```
plugins/ironmanplanner/
  IronmanPlannerPlugin.java     @PluginDescriptor(name = "Ironman Planner", tags = {"ironman","tracker"})
  IronmanPlannerConfig.java     serverUrl, ingestToken, sendBank, sendPosition, intervalSeconds
  PlannerClient.java            OkHttp (já no RuneLite), manifest GET, submit POST com Gson
  PlayerStateCollector.java     lê skills, quests, varbits do manifest, clog; calcula delta vs último envio
  PlannerPanel.java (opcional)  painel lateral com "próximo step" vindo de GET /api/now
```

Regras do AGENTS.md do Microbot que importam aqui: tudo que lê o `Client` roda na thread do cliente
(`Microbot.getClientThread().runOnClientThreadOptional` ou `invokeLater`), rede nunca na thread do
cliente, e scripts em `ScheduledExecutorService` próprio.

## Bônus que só o Microbot permite

- Painel in-client com o próximo step do guia e botão "feito", conversando com `PATCH /api/steps/:id`.
- Overlay de "itens que o guia manda guardar" ao abrir o banco, comparando com `data/guide` `items`.
- Detecção de contexto: se o jogador está perto do NPC ou região de um step pendente, destacar na UI web.
- Em fase bem posterior, opcional e separado: scripts de rotina (ex.: ir ao banco e pegar o inventário
  do próximo step) usando `Rs2Walker` e `Rs2Bank`. Fora do escopo do planner.

## Impacto nos outros docs

- Doc 03: F8 (plugin próprio) passa de "S v1, M v2" para "M MVP". F7 (WikiSync) cai para "C" ou sai.
- Doc 04, seção 7.4: as tarefas 16 e 17 (ingest endpoint e plugin v0) sobem para logo depois da 8
  (adapter de hiscores), antes do dashboard.
- Doc 01, decisão 2 (WikiSync no MVP): resolvida pelo plugin no Microbot.
