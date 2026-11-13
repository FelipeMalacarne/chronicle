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
| Skills (level real, xp, boosted) | `client.getRealSkillLevel`, `getSkillExperience`, evento `StatChanged` | no evento (1.1.1: debounce de 1 s; XP e boost sem teto, level-up com teto de 3 s) |
| Quests (NOT_STARTED, IN_PROGRESS, FINISHED) | `Quest.values()` + `quest.getState(client)` na thread do cliente, enviados na faceta `data.quest` do contrato v1 (doc 04, seção 6.4) com a chave `quest.getName()`. Feito no plugin 1.1.0 (adendo "contrato do item 7" abaixo) | todos no `full`; em `delta` só os que mudaram |
| Diaries por tier, quest points, unlocks | genérico: todo varbit/varp que o manifest listar, em `data.varb`/`data.varp`. Hoje o manifest pede os 48 varbits `DIARY_<ÁREA>_<TIER>_COMPLETED` e o varp 101 (`QUEST_POINTS`) de `content/reference/varbits.json`, mais os `proof` de `unlocks.json` quando existirem; o servidor deriva diary, `qp` e unlock. O plugin 1.0.0 já lê o que o manifest pede | em `VarbitChanged` (1.1.1: debounce de 1 s, teto de 3 s para os vars do manifest) ou na verificação de 10 s |
| Combat achievements | varbits/cs2 do manifest | em `VarbitChanged` |
| Collection log | contadores pelos varps do manifest; os itens, página por página, lidos da interface aberta em `data.collectionLog` (1.12.0, adendo de 2026-11-13). O `collectionLogSlots` planejado aqui nunca foi enviado | os contadores como qualquer var; cada página quando o jogador a abre |
| Banco, inventário, equipamento | `ItemContainerChanged` para `live`; cache `Rs2Bank`/`Rs2Inventory` apenas como `restored` | ao fechar o banco, ao logar; opt-in separado por container |
| Posição, mundo, membro | `client.getLocalPlayer().getWorldLocation()`, `getWorld()` | a cada N segundos, opt-in; útil para "você está em Ardougne, próximo step aqui" |
| Quest Helper ativo | não necessário | |

Protocolo: idêntico ao doc 04, seção 6.4. `GET /api/ingest/manifest` devolve
`contract_version: 1`, `manifest_version`, varbits/varps/coleções, `token_scopes`, permissões de
itens e `resync`. `POST /api/ingest` envia `client_seq` monotônico e `kind: full|delta`; a resposta
repete `accepted_seq`, a versão e se exige `resync`. Hoje o token `ipl_` fica por conta na config; a auth
planejada (adendo de 2026-11-05 abaixo) dispensa o token em dev e usa o device flow do Zitadel em `oidc`.

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

## Adendo de 2026-11-09: um só plugin do Chronicle

Decisão do dono: um só plugin do Chronicle entre os plugins do Microbot. A ponte de controle do doc 08 (menus
de NPC, objeto e equipamento, orações, feitiços e as ações tipadas deles) e o export das opções de
config saíram do Dev Bootstrap para o `fm/ironmanplanner` 1.3.0. Eles só sobem no cliente de dev do
harness (`-Dmicrobot.dev.harness=true`, o do `mb-client`) e só respondem com o token do Agent
Server; no cliente do dia a dia o plugin continua só lendo e nada disso existe. O tracking (o
escopo abaixo) segue somente leitura em qualquer cliente.

## Adendo de 2026-11-09: o plugin se chama Chronicle Link

O plugin passou a se chamar Chronicle Link (`fm/chroniclelink`, `ChronicleLinkPlugin`, versão 1.4.0):
"[FM] Chronicle Link" na lista do RuneLite e "Chronicle" no botão da barra, no overlay e no painel.
O grupo de config continua `fm-ironmanplanner`, para a URL, o token e a conta do dono sobreviverem,
e nada do contrato de ingest nomeia o plugin, então o servidor não mudou. O `mb-install` remove o
`IronmanPlannerPlugin.jar` antigo ao instalar o novo, nos dois clientes. As seções abaixo descrevem a
versão 1.0 com o nome antigo e ficam como registro.

## Adendo de 2026-11-12: o plugin mora em `microbot/`

O Chronicle Link, os outros plugins e o harness do cliente de dev saíram do repositório
`microbot-plugins` para `microbot/` neste repositório, com o histórico (decisão 35 do doc 01). O
pacote continua `net.runelite.client.plugins.microbot.fm.chroniclelink`. Os jars saem de `just plugins`
(`gradle -p microbot pluginJars`, em `microbot/build/plugins`) e os testes de `just test-microbot`, que
o job `nix` do CI roda. O harness (`mb-client`, `mb-deploy`, `mb-install`, `mb-test`, `mb`) fica em
`microbot/bin`, no PATH do dev shell; as regras e a segurança estão em `microbot/AGENTS.md`. Um
contrato que muda dos dois lados (ingest, ponte, variáveis do Chronicle Link) muda num commit só. Os
adendos com `microbot-plugins` são de antes disso.

## Adendo de 2026-11-04: plugin implementado

O plugin foi implementado em `FelipeMalacarne/microbot-plugins` (hoje `microbot/`), pacote
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

## Adendo de 2026-11-04: contrato do item 7 (quests e vars de referência)

O servidor do item 7 está pronto (doc 04, seções 6.4 e 9); este adendo é tudo que o plugin 1.1.0
precisa. O contrato continua `contract_version: 1`: os campos novos são opcionais e o servidor aceita o
plugin 1.0.0 como está.

**Decisão do dono (2026-11-04):** o plugin lê **genericamente** todo varbit e varp que o manifest
listar e manda os valores; ele não sabe o que é diary, quest point ou unlock. Fatos novos que vivem em
vars (unlocks com `proof` em `content/reference/unlocks.json`, combat achievements) pedem só catálogo e
servidor, nunca uma versão nova do plugin.

### O que muda no manifest

`GET /api/ingest/manifest` passa a devolver:

- `varbits`: os varbits de `content/reference/varbits.json` (os 48 tiers de diary), os `proof` do tipo
  `varbit` de `unlocks.json` (hoje nenhum) e os de regras `varbit` do conteúdo, sem repetição e
  ordenados;
- `varps`: idem para varps; hoje `[101]` (`QUEST_POINTS`);
- `quests: true`: o servidor aceita `data.quest`. **Só envie `data.quest` quando `quests` for `true`**;
  um servidor anterior não manda o campo (Gson: `null`/`false`) e rejeitaria `data.quest` como campo
  desconhecido;
- `items.only_item_ids` não vem mais preenchido: mande o container inteiro (o filtro do
  `ItemQuantities` continua valendo se um dia vier).

O `manifest_version` muda com isso, então o primeiro push de cada token após o deploy recebe `409` com
`resync:true` e o plugin manda `full`, como já faz.

```json
{
  "contract_version": 1,
  "manifest_version": 329934161,
  "token_scopes": ["state:write", "items:bank"],
  "varbits": [3578, 3599, 3611, 4458, 4459, 4460, 4461, 4462, 4463, 4464, 4465, 4466, 4467, 4468,
              4469, 4471, 4472, 4473, 4474, 4475, 4476, 4477, 4478, 4479, 4480, 4481, 4482, 4483,
              4484, 4485, 4486, 4487, 4488, 4489, 4490, 4491, 4492, 4493, 4494, 4495, 4496, 4497,
              4498, 4566, 7925, 7926, 7927, 7928],
  "varps": [101],
  "collections": [],
  "skills": true,
  "quests": true,
  "items": {"bank": true, "inventory": false, "equipment": false},
  "interval_seconds": 10,
  "resync": true
}
```

Este é o manifest real do conteúdo atual (`BuildDefinition` com o catálogo de 2026-11-04); a versão
muda quando o catálogo ou as regras mudarem, e o plugin nunca a interpreta, só a repete no push.

### O que muda no push

- `data.varb` e `data.varp` continuam como estão (`Map<String, Long>`, chave decimal): para cada id de
  `manifest.varbits`, `client.getVarbitValue(id)`; para cada id de `manifest.varps`,
  `client.getVarpValue(id)`. Em `full`, todos os ids do manifest; em `delta`, só os que mudaram.
- Novo `data.quest` (`Map<String, String>`): para cada `Quest q : Quest.values()`, chave `q.getName()`
  (o nome exato que o RuneLite expõe, por exemplo `"Recipe for Disaster - Mountain Dwarf"`), valor
  `q.getState(client).name()` (`NOT_STARTED`, `IN_PROGRESS` ou `FINISHED`). Lido na thread do cliente
  (`Quest.getState` roda script do cliente). Em `full`, todas as quests; em `delta`, só as que mudaram de
  estado desde o último ACK. Mapa vazio é `{}`, nunca `null`; nome sem espaços nas pontas e com até 120
  bytes.

```json
{
  "contract_version": 1,
  "username": "Example Iron",
  "profile": "STANDARD",
  "account_type": "IRONMAN",
  "manifest_version": 329934161,
  "client_seq": 1,
  "kind": "full",
  "sent_at": "2026-11-04T12:00:00Z",
  "data": {
    "varb": {"3578": 0, "3599": 0, "4458": 1, "4459": 0},
    "varp": {"101": 33},
    "level": {"attack": 40, "agility": 30},
    "xp": {"attack": 37224, "agility": 13363},
    "boosted": {"attack": 40, "agility": 30},
    "quest": {
      "Cook's Assistant": "FINISHED",
      "X Marks the Spot": "FINISHED",
      "Rune Mysteries": "IN_PROGRESS",
      "Recipe for Disaster - Mountain Dwarf": "NOT_STARTED"
    },
    "items": {
      "bank": {"state": "restored", "observed_at": "2026-11-04T11:55:00Z", "revision": 41, "quantities": {"995": 250000}},
      "inventory": {"state": "unavailable", "observed_at": "2026-11-04T12:00:00Z"},
      "equipment": {"state": "unavailable", "observed_at": "2026-11-04T12:00:00Z"}
    }
  }
}
```

Um `delta` depois de terminar Rune Mysteries:

```json
{
  "contract_version": 1, "username": "Example Iron", "profile": "STANDARD",
  "manifest_version": 329934161, "client_seq": 2, "kind": "delta", "sent_at": "2026-11-04T12:10:00Z",
  "data": {"varp": {"101": 34}, "quest": {"Rune Mysteries": "FINISHED"}}
}
```

O que o servidor faz com isso: resolve cada nome pelo registry (nome canônico, aliases do RuneLite e do
WikiSync) e grava `quest` com o nome da wiki; nome desconhecido é descartado, não rejeita o push (fica
no `raw` do snapshot para reprocessar). Deriva `diary` de cada varbit de diary presente (`1` = tier
completa) e avalia `qp>=N` pelo varp 101. Um `full` sem `data.quest` (plugin 1.0.0) não apaga as quests
já guardadas; um `full` com `data.quest` substitui todas.

Para o plugin: `IngestContract.Manifest` ganha `Boolean quests`; `IngestContract.PushData` ganha
`Map<String, String> quest`; o `StateCollector` lê as quests; o `PushBuilder` calcula o delta de
quests como faz com os varbits. Os 216 nomes do `Quest.java` pinado em `data/samples/runelite`
resolvem no catálogo (teste `TestRuneLiteQuestNamesResolve`); se o cliente ganhar quests novas antes do
catálogo, elas ficam `unknown` até o `catalogbuild` alcançar.

### Nomes reservados para a 1.2.0 (não implementar agora)

O servidor ainda rejeita estes campos (`DisallowUnknownFields`); eles ficam reservados para o contrato
continuar v1 sem colisão. Cada um terá uma flag no manifest, como `quests`, e o plugin só manda quando
ela vier `true`:

| Campo do push | Flag do manifest | Conteúdo previsto |
|---|---|---|
| `data.position` | `position` | posição e mundo, opt-in no cliente: `{"world": 330, "region_id": 10547, "x": 2662, "y": 3305, "plane": 0}` |
| `data.collectionLogSlots`, `data.collectionLogItemCount` | `collections` (já existe, lista não vazia) | já no contrato v1; o plugin passa a enviar |
| `data.ca` | `combat_achievements` | só se os varps genéricos não bastarem; a preferência é ler CA pelos varps que o manifest listar, sem campo novo |

Combat achievements e unlocks, quando o catálogo tiver os vars, chegam pelos `varbits`/`varps` do
manifest, sem mudar o plugin.

**Feito em 2026-11-10 para os combat achievements e o contador do collection log**, sem mudar o
plugin: o manifest passou a pedir os 21 varps `CA_TASK_COMPLETED_<n>` (cada task é o bit `id%32` do
varp `id/32`), os varbits `CA_TOTAL_TASKS_COMPLETED_<TIER>` e os varps `COLLECTION_COUNT` e
`COLLECTION_COUNT_MAX`. O `data.ca` reservado não é necessário. O collection log por item
(`collectionLogSlots`) continua sem envio: o jogo só diz quais itens você tem com o log aberto.

## Adendo de 2026-11-05: em uso, push por evento, painel da rota e auth

**Em uso no zaros.** O plugin 1.1.0 rodou no cliente de dev com a conta do dono: o primeiro `full` levou as
217 quests do cliente 2.6.52 (todas resolvidas no catálogo, Architectural Alliance inclusive), os 48
varbits de diary e o varp 101. No planner, 54 objetivos de quest e 2 de diary viraram `met` com evidência
e os objetivos de item saíram de `unknown` depois de abrir o banco na sessão.

**1.1.1: captura por evento** (decisão 26 do doc 01; branch `feat/push-por-evento`). Eventos marcam uma
captura com debounce de 1 s depois do último evento e, nos eventos "duros", teto de 3 s depois do
primeiro:

| Evento | Classe |
|---|---|
| `ItemContainerChanged` de banco, inventário e equipamento, só com flag do manifest, scope e toggle | duro |
| `VarbitChanged` de um var do manifest, ou do varp 101 com `quests` ligado | duro |
| `VarbitChanged` de outro var, com `quests` ligado (marca as quests para reler) | suave |
| `StatChanged` com level real novo | duro |
| `StatChanged` só de XP ou boost | suave |
| fim da espera de 3 ticks depois do login ou da troca de mundo | duro |

O protocolo não muda: um push por vez, ACK antes do próximo delta, mesmo `client_seq` no retry, backoff,
409 com renumeração e `full`. Pedidos durante um push em voo ou um backoff viram uma só captura depois
dele; a leitura periódica (`interval_seconds`) é pulada quando uma captura por evento acabou de rodar.
Captura continua na thread do cliente; o gatilho só registra horários.

**1.2.0: painel da rota** (decisão 27 do doc 01; item 14 do doc 04, seção 9). Painel lateral do RuneLite,
no visual dele (sem canvas): a rota ativa a partir do cursor, o passo atual destacado, os próximos, os
bloqueados com cadeado e o "falta …", os chips de objetivo com veredito, os itens do trecho e os botões
Feito, Pular e Desfazer (as mesmas marcas da web). Atualiza depois de cada push confirmado (a resposta do
ingest passa a trazer o cursor e os passos que mudaram) e depois de cada marca. O servidor continua a
fonte da verdade; o plugin guarda o último estado só para exibir quando o servidor estiver fora.

**Auth do plugin** (decisão 28 do doc 01; doc 04, seção 2.5). Em `AUTH_MODE=dev` o plugin precisa só da
URL: ingest, manifest e as rotas do painel são aceitos sem token vindos de `127.0.0.1`. Em `oidc`, Device
Authorization Grant no Zitadel (aplicação nativa pública, sem DCR): o plugin descobre issuer e
`client_id` no planner, mostra o código, guarda o refresh token na config e renova sozinho; as roles do
projeto no JWT (`ingest`, `items:*`, `marks:write`) fazem o papel dos `scopes`. A credencial é plugável no
plugin desde o painel (nenhuma em dev, device flow em `oidc`), e o `ipl_` sai quando o device flow entrar.

## Adendo de 2026-11-05: painel da rota e auth sem token implementados (item 14)

Servidor no branch `feat/painel-plugin`, plugin 1.2.0 no `feat/painel-rota` do `microbot-plugins`.
Contrato e rotas: doc 04, seções 2.5 e 6.4; espelho em `docs/arquitetura/types.ts`.

**Credencial plugável no plugin.** `PlannerCredential` decide o que o pedido leva:

- `NoCredential` (padrão, "None (dev, URL only)"): nada de `Authorization`; `X-Ironman-Installation`
  com o id da instalação e, se preenchido, `X-Ironman-Account` com a "Conta". Só funciona com o
  planner em `AUTH_MODE=dev` e o plugin chegando pelo loopback (`just dev` no zaros; na stack do
  compose o peer é o gateway do Docker e o plugin precisa do token).
- `TokenCredential` ("ipl_ token"): `Authorization: Bearer ipl_…`, como na 1.1.1. As marcas do painel
  pedem o escopo `marks:write` no token.
- Device flow de `oidc` (item 15): só o ponto de extensão documentado na interface, sem implementação
  (feito na 1.9.0: adendo de 2026-11-11).

O id da instalação é um UUID aleatório gerado uma vez e guardado na config (`installationId`,
escondida). Ele identifica a instalação, não o jogador: o servidor guarda o `client_seq` e o manifest
por `(conta, instalação)` (migration 0006), então trocar de personagem no mesmo cliente não mistura
sequências. Token, username e RSN continuam fora dos logs.

**Qual conta.** O push já leva o nome do personagem (`username`), então o contrato não ganhou campo:
o servidor usa a "Conta" quando preenchida, senão a conta desse RSN, senão a única conta; no manifest e
no painel (sem RSN), a conta da última vez que a instalação enviou. Com várias contas e nenhum desses
sinais, 422 pedindo a "Conta".

**Painel** (`RoutePanel`, `RoutePanelController`, `PanelText`). Botão "Ironman Planner" na barra
lateral; dentro, no visual do RuneLite:

- cabeçalho: nome da rota, atribuição do guia (regra 6), "N de M feitos · K bloqueados" e a linha de
  conexão ("Conectado · há 12 s"; "Desatualizado · há 3 min · servidor fora do ar"; "Sem conexão");
- AGORA: referência e seção, texto completo, local, chips de objetivo com glifo por veredito (✓ met,
  ✗ unmet, ? unknown com borda tracejada; "requer"/"usa" no rótulo), cadeado e "falta …" quando
  bloqueado, botões Feito, Pular e Desfazer;
- itens do trecho do cursor (✓ na mochila/equipado, ○ no banco, ✗ falta, ? não sabemos) e as ações de
  banco do guia; sem setup no guia, "Itens dos próximos passos" com o aviso de prévia;
- Próximos: os 15 passos seguintes numa lista compacta com glifo de estado, cadeado nos bloqueados;
  clique expande texto, chips e Feito/Pular daquele passo;
- botão Atualizar.

Feito e Pular chamam `PATCH /api/plugin/guides/{guide}/steps/{id}/mark` (`manual_done`, `skipped`);
Desfazer desfaz a última marca do painel com a marca anterior do passo (`clear` se não havia, o mesmo
`kind` se havia `skipped`/`manual_done`/`pinned_open`, e o restore em lote quando era `implied` e a
ação foi Feito; Pular num presumido se desfaz com `clear`, porque o restore só volta por cima de
`manual_done`). As respostas já trazem a janela nova e o servidor publica o `progress`/`owner` do SSE,
então a web atualiza junto.

**Quando relê.** Depois de cada push confirmado cuja `route` mudou (cursor diferente do exibido, outra
rota ativa ou `changed` não vazio; com servidor antigo, `steps_changed > 0`), depois de cada marca, no
login, ao abrir o painel e a cada 30 s enquanto ele está visível. A última janela fica em memória e
continua na tela, marcada "Desatualizado", quando o servidor não responde; trocar URL ou credencial
descarta o cache e o Desfazer.

**Threads.** Captura na thread do cliente, protocolo de ingest na `ironman-planner-sync`, HTTP do
painel na `ironman-planner-panel`, Swing só no EDT. Nenhum HTTP no EDT nem na thread do cliente.

Verificação: 92 testes JUnit (janela e resposta do ingest com campos novos e antigos, chamadas HTTP de
Feito/Pular/Desfazer, gatilhos de releitura, cache offline, credenciais, id de instalação, textos) e o
jar; render sem tela do painel conferido. Falta instalar e usar no jogo.

## Adendo de 2026-11-10: relógios e posição (Chronicle Link 1.8.0)

Contrato v1, campos opcionais com flag no manifest, como `quests` (spec no doc 04, seção 6.4):

| Campo do push | Flag do manifest | Conteúdo |
|---|---|---|
| `data.timers` | `timers` | as abas do Time Tracking do RuneLite (`BIRD_HOUSE`, cada aba de farming, `FARMING_CONTRACT`) com o resumo e, crescendo ou pronto, `readyAt` em segundos; lidas dos rastreadores do próprio plugin Time Tracking (a mesma leitura da ponte, `TimeTrackingClocks`); nada com ele desligado |
| `data.position` | `position` | `{"world", "x", "y", "plane"}`, só com "Send position" ligado (desligado por padrão); sozinha, vai num push a 16 tiles ou em outro mundo ou plano, e pega carona em qualquer outro push |

O servidor grava as facetas `timer` (os Recorrentes do Agora, `GET /api/account/timers`; decisão 31 do
doc 01) e `position` (só a última, sem histórico). Um plugin 1.7 ignora as flags novas; um servidor
anterior não as anuncia e o 1.8 não manda os campos. Verificação: testes JUnit (`readyAt` só crescendo
ou pronto, `full` e `delta` dos relógios, aba nova pede `full`, limiar da posição; 111 no pacote) e,
no servidor, testes de contrato e `TestTimersAndPositionPostgres`.

## Adendo de 2026-11-11: login do plugin em `oidc` (Chronicle Link 1.9.0, item 15)

Implementa a auth de `oidc` da decisão 28 do doc 01 (doc 04, seção 2.5), antes do deploy no saradomin
(item 18). O contrato de ingest não muda: muda só a credencial.

**No plugin.** Nova opção de Credential, "Login (Chronicle panel)" (`LoginCredential`, `LoginSession`,
`DeviceLogin`). Com ela, o painel lateral mostra **Entrar** enquanto não há login:

1. `GET /api/auth/client` do planner dá issuer, `client_id` da aplicação nativa e scopes; o
   `/.well-known/openid-configuration` do issuer dá os endpoints de device e token.
2. Device Authorization Grant (RFC 8628): o painel mostra o código e **Abrir página** abre a página
   do issuer com o código preenchido; o plugin consulta o token no intervalo pedido (`slow_down` soma
   5 s) até a confirmação, a recusa ou o vencimento do código.
3. O refresh token fica na config do RuneLite (`loginRefreshToken`, escondida e secreta); o access
   token (JWT) fica só em memória e é renovado dois minutos antes de vencer, ou logo depois de um 401
   do planner. Refresh recusado (`invalid_grant`) desfaz o login; issuer fora do ar mantém o login e
   tenta de novo a cada 30 s. **Sair**, no rodapé do painel, apaga o refresh token.

Cada pedido leva `Authorization: Bearer <jwt>`, `X-Ironman-Installation` e, se preenchida,
`X-Ironman-Account`. A impressão da credencial não muda quando o token é renovado, então o
`client_seq` segue.

**No servidor.** Em `AUTH_MODE=oidc`, um bearer que não começa com `ipl_` nas rotas do plugin
(manifest, ingest e painel) é verificado como o da CLI (JWKS, `iss`, `aud` com a aplicação nativa,
`exp`, política de `sub`/role, usuário de `(issuer, sub)` já existente). Ele age pela instalação como
esse usuário: a conta sai da "Conta", do personagem do push ou da última conta da instalação, sempre
entre as contas ativas desse usuário (outra conta é 404), e o registro da instalação fica em
`plugin_ingest_tokens` como o de dev (`label` `oidc installation`). As roles do projeto viram os
`scopes`: `ingest` → `state:write`, e `items:bank`, `items:inventory`, `items:equipment` e
`marks:write` com o mesmo nome; sem `ingest` é 403. Os tokens `ipl_` continuam aceitos em qualquer modo
(nenhum é emitido em `oidc`, já que o bootstrap é só de dev).

Verificação: `TestOIDCPluginLogin` (JWT do provedor falso nas rotas do plugin, roles → scopes, sem
`ingest`, sem instalação, vencido, `ipl_` à parte), `TestOIDCInstallationPostgres` (só as contas do
usuário, escopos das roles no push), `LoginSessionTest` no plugin (fluxo completo contra um issuer falso,
renovação com rotação, `invalid_grant`, issuer fora do ar) e o painel renderizado sem cliente.

## Adendo de 2026-11-13: collection log por página, versão da ponte e break (Chronicle Link 1.12.0)

Item 16 do doc 04, seção 9. O jogo só diz quais itens do collection log a conta tem com o log aberto,
página por página. Então o plugin lê cada página que o jogador abre e manda a página inteira.

**De onde vêm os ids.** Nada foi adivinhado: o `ChatCommandsPlugin` do próprio cliente pinado (2.6.52)
lê a página "All Pets" do mesmo jeito, e os valores dele (lidos do bytecode do jar) batem com o
`InterfaceID` do vendor:

- depois do script `COLLECTION_DRAW_LIST` (2731, evento `ScriptPostFired`), o primeiro filho de
  `InterfaceID.Collection.HEADER_TEXT` (40697876) é o título da página;
- os filhos de `InterfaceID.Collection.ITEMS_CONTENTS` (40697893) são os espaços da página, com o
  item e a quantidade; o obtido é desenhado com opacidade 0;
- o mesmo plugin não lê o log de outro jogador aberto pelo adventure log de uma casa. A interface é
  o grupo 947 (`MENU_NEW`) ou 187 (`MENU`), o título "The Exploits of <nome>" fica em
  `MenuNew.TITLE` ou `Menu.LJ_LAYER2` e é lido no tick seguinte ao carregamento. O dono vale até o
  próximo `LOADING` ou `HOPPING`. O Chronicle Link faz igual e, por cautela, também não lê enquanto
  o dono ainda não foi lido.

**A aba** (Bosses, Raids, Clues, Minigames, Other) é a da única lista de aba visível
(`BOSS_CONTAINER`, `RAID_CONTAINER`, `CLUE_CONTAINER`, `MINIGAME_CONTAINER`, `OTHER_CONTAINER`). Com
nenhuma ou mais de uma visível, a aba fica vazia, nunca adivinhada.

**Contrato v1, campo opcional com flag no manifest**, como `timers` (spec no doc 04, seção 6.4):

| Campo do push | Flag do manifest | Conteúdo |
|---|---|---|
| `data.collectionLog` | `collection_log` | título da página → `{"tab", "observed_at", "items"}`. `tab` é `BOSSES`, `RAIDS`, `CLUES`, `MINIGAMES` ou `OTHER`, ausente quando não se sabe. `observed_at` é quando a página foi lida. `items` é cada espaço da página: id do item → quantos obtidos, `0` para o que falta |

```json
"collectionLog": {"Barrows Chests": {"tab": "BOSSES", "observed_at": "2026-11-13T12:00:00Z",
                                     "items": {"4708": 1, "4710": 0, "4712": 2}}}
```

Regras:

- Cada página presente substitui a guardada, tanto num `full` quanto num `delta`.
- As páginas ausentes ficam como estão: o plugin só conhece as páginas abertas no jogo. Um `full`
  sem o campo não apaga nada.
- O plugin guarda as páginas lidas no login atual. O `full` leva todas; o `delta` leva as novas e as
  que mudaram. Página reaberta sem mudança não é reenviada, e página nova não pede `full`.
- A leitura de uma página nova ou mudada pede captura (evento duro, como um container).
- Limites do servidor: 500 páginas, 1.000 espaços por página e título de até 80 bytes. O plugin não
  manda a página que passa deles.

**No servidor.** Cada página vira um fato da faceta `collection_log`, com chave `page:<título>` e
valor `{"tab", "items"}`, observado quando a página foi lida. A leitura mais velha nunca substitui a
mais nova, e o `observed_at` fica limitado ao relógio do servidor. O bitset `collectionLogSlots`, que
nenhum plugin manda, passa a substituir só os fatos dele (chave = id do item), sem apagar as páginas.

O `GET /api/account/state` ganha `collection_log_pages`: por aba e título, os itens obtidos (com
quantos), os que faltam, os totais e a fonte com a idade. Página nunca aberta não aparece, então os
itens dela ficam desconhecidos, nunca "faltando". Os contadores do jogo (`collection_log`) continuam
à parte, pelos varps. A tela Conta ainda não mostra as páginas: o desenho vai para o canvas antes
(regra 11).

Um plugin 1.11 ignora a flag nova. Um servidor anterior não a anuncia, e o 1.12 não manda o campo. A
`manifest_version` muda, então os plugins em uso buscam o manifest de novo e mandam um `full`.

**Versão da ponte.** A ponte de controle ganha `GET /bridge/version` (`{"version": "1.12.0"}`, mesmo
token e checagens das outras rotas). O agent lê a versão a cada `status` com o cliente rodando e a
devolve em `bridge`; fica vazio sem ponte, com o plugin desligado ou numa versão anterior à 1.12. O
`Hello.Bridge` continua vazio: o hello sai antes de o cliente subir no worker (doc 11, seção 14.3).

**Break do Microbot.** A ponte ganha `GET /bridge/break` (mesmas checagens), só leitura, para o "Até a
pausa" da web v7. Os valores vêm dos campos estáticos do cliente pinado:

- `active`: o mesmo que o `ClientAlertReporter` do cliente chama de break. Vale o break do Break
  Handler (`BreakHandlerScript.isBreakActive()`, qualquer estado fora de esperar), o micro break do
  antiban (`isMicroBreakActive()`) ou o break do Break Handler V2
  (`BreakHandlerV2State.isBreakActive()`).
- `in_s`: segundos até o próximo break (`BreakHandlerScript.breakIn`), só fora de um break.
- `left_s`: segundos que faltam no break atual (`breakDuration`), só num break do Break Handler.

Os dois tempos só saem com o plugin do Break Handler ligado e sem o `-1` de "não definido": os
campos são estáticos e o `breakIn` sobrevive ao plugin desligado. O Break Handler V2 guarda os tempos
na instância do plugin, então com ele só `active` é conhecido. O agent lê `/bridge/break` no `status`
(só quando a ponte respondeu a versão) e devolve `break: {active, in_s, left_s}`, ausente quando a
ponte não responde.

**Verificação:**

- JUnit (`CollectionLogTest`, `AgentBridgeTest`): os ids iguais aos do `ChatCommandsPlugin`, título
  sem tags, espaço obtido só com opacidade 0, aba só com uma lista visível, dono do adventure log,
  páginas por login sem reenviar a mesma, `full` e `delta` das páginas, as rotas de versão e de break
  atrás do token e o estado do break (tempos só com o Break Handler ligado e fora do `-1`). São 143
  testes JUnit em `microbot/`, todos passando, e o `pluginJars`.
- No servidor: testes de tabela da validação, dos fatos e do relógio, e
  `TestCollectionLogPagesPostgres` (substituição por página, `full` sem o campo, leitura velha, o
  bitset sem apagar páginas, id fora do int4).
- `TestCollectionLogAccountStatePostgres`, do push até a Conta.
- `TestStatusReadsTheBridgeVersion` e `TestParseBreak` no agent.

**Não conferido no jogo:**

- abrir o log e ver as páginas chegarem;
- a aba pela lista visível;
- o que o cabeçalho mostra durante a busca do log (Search): se mostrar outro título, o plugin grava
  uma página com esse nome;
- a quantidade de um item obtido mais de uma vez;
- o adventure log de outro jogador numa casa;
- o `/bridge/break` com o Break Handler (e o V2) num break de verdade.

## Adendo de 2026-11-13: gravação por demonstração (Chronicle Link 1.13.0, fase 2 do doc 10)

Decisão E8 do doc 10: o plugin passa a registrar as interações do dono, ainda sem agir, e só com
opt-in. É a matéria-prima das tarefas da fase 2 (doc 10, seção 11): o dono joga um trecho da rota
com a gravação ligada, o servidor guarda os eventos e o agente da extração os transforma em tarefas
propostas para um passo, que só entram no conteúdo com a confirmação do dono (regra 9).

**No plugin.** Nova opção "Record demonstration", desligada por padrão. Com ela ligada e o manifest
anunciando `recording`, o `DemonstrationRecorder` registra, com os ids do jogo:

| Tipo | Quando | Campos |
|---|---|---|
| `move` | primeira amostra da gravação; outro mundo, plano ou região do mapa; e onde o personagem parou (2 ticks parado) depois de andar 5 tiles ou mais desde a última amostra | só a posição |
| `npc` | opção de menu clicada num NPC (`MenuOptionClicked`, `NPC_*_OPTION`) | `npc_id`, `option`, `name` |
| `object` | opção de menu clicada num objeto (`GAME_OBJECT_*_OPTION`) | `object_id`, `option`, `name`, `target_x`/`target_y` (o tile do objeto) |
| `dialogue` | opção de diálogo escolhida: clicada (`WIDGET_CONTINUE` em `Chatmenu.OPTIONS`) ou pela tecla numérica (o script do ouvinte de teclas da interface, `ScriptPreFired`); uma escolha por tela de opções | `option` (o texto), `npc_id` do último NPC clicado |
| `shop_buy` | item que entrou no inventário com uma loja aberta (`SHOPMAIN`), menos coins | `item_id`, `qty`, `npc_id` do lojista clicado antes |
| `bank_open` | a interface do banco abriu (`BANKMAIN`) | `npc_id` ou `object_id` clicado antes |
| `bank_withdraw`, `bank_deposit` | o que saiu ou entrou no banco enquanto aberto: a diferença do inventário e do equipamento somados, então equipar não conta | `item_id`, `qty` |
| `item_use` | item do inventário usado num NPC, objeto ou outro item (`WIDGET_TARGET_ON_*`) | `item_id`, um de `npc_id`/`object_id`/`target_item_id` |
| `item_op` | opção do próprio item no inventário (Rub, Break, Read; não Drop nem Examine) | `item_id`, `option` |

Toda entrada leva `recording` (um id por login com a gravação ligada, `rec-<uuid>`), `seq` (de 1, por
gravação), `at` e a posição do personagem (`world`, `x`, `y`, `plane`). Nunca chat, outros jogadores,
teclas fora das opções de diálogo, magias ou o caminho tile a tile. O overlay mostra "● Gravando" e
quantos eventos esperam o servidor.

**No contrato** (v1, campo opcional com flag no manifest, como `collection_log`; spec no doc 04,
seção 6.4):

| Campo do push | Flag do manifest | Conteúdo |
|---|---|---|
| `data.recording` | `recording` | lista dos eventos ainda não confirmados, do mais velho ao mais novo, em `full` e em `delta` (no máximo 200 por push; o servidor aceita até 500) |

```json
"recording": [{"recording": "rec-5b1c", "seq": 3, "at": "2026-11-13T12:00:00Z", "kind": "object",
               "world": 330, "x": 3222, "y": 3218, "plane": 0, "object_id": 10583,
               "target_x": 3208, "target_y": 3220, "option": "Bank", "name": "Bank booth"}]
```

Regras:

- O plugin guarda os eventos em memória (até 5.000; depois disso os novos são descartados e contados)
  até um push que os levou ser confirmado. Um push que estourou o tempo ou levou 409 reenvia os mesmos
  eventos, e o servidor guarda cada `(recording, seq)` uma vez só.
- Os eventos só vão para a conta do personagem que os gravou: outro personagem logado descarta os
  pendentes do anterior. Desligar a opção ou deslogar para a gravação, mas o que estava pendente ainda
  é enviado.
- Eventos nunca pedem `full` nem entram no cálculo do delta: cada push leva os pendentes.

**No servidor.** O ingest valida os eventos (`internal/core/recording`: tipos da lista fechada, só os
campos de cada tipo, ids não negativos, texto sem tags e com até 120 bytes, `at` limitado ao relógio
do servidor) e os grava em `recorded_events` (migration 0017), na mesma transação do push e sempre como
o único escritor. O `raw` do snapshot não guarda `data.recording`, então apagar a gravação a apaga de
vez. O dono lê com `GET /api/account/recording?account=&since=&until=&limit=` e apaga com
`DELETE /api/account/recording?account=&recording=`, as duas atrás de `authed` + `accountParam`
(regra 10); na CLI, `chronicle recording [--since 2h] [-o arquivo]` e `chronicle recording delete --yes`.

**Privacidade, decisões conservadoras** (pendentes do dono): só o que o dono faz, nada de outros
jogadores ou chat; opt-in, desligado por padrão; a gravação fica até o dono apagar (não há expiração
automática, por falta de decisão). Ver o doc 10, seção 14 (E11 a E13).

Um plugin 1.12 ignora a flag nova; um servidor anterior não a anuncia e o 1.13 não manda o campo. A
`manifest_version` muda, então os plugins em uso buscam o manifest de novo e mandam um `full`.

**Verificação:** JUnit (`DemonstrationTest`: amostras de posição, ids e textos de cada tipo, a escolha
de diálogo uma vez por tela, banco e loja pela diferença, a fila por personagem com confirmação e
limite, o classificador de menu e o JSON igual ao do `TestWireShape` do servidor), 155 testes no
pacote; no servidor, a tabela de validação de `recording`, o portão do manifest, o relógio,
`TestRecordingPostgres` (um evento por `(recording, seq)` com reenvio, o `raw` sem a gravação, leitura
paginada e apagar) e as rotas (dev, oidc com CSRF no DELETE e fora dos dois).

**Não conferido no jogo:** a escolha de diálogo pela tecla (o evento do script do ouvinte de teclas),
as diferenças do banco e da loja com a interface real, o tile do objeto numa instância e o volume de
eventos de uma sessão longa.

## Adendo de 2026-11-13: modo assistido no painel (Chronicle Link 1.14.0, fase 2 do doc 10)

O painel lateral passa a mostrar, no card AGORA, as tarefas do passo atual (doc 10, seção 7) como uma
lista de verificação, entre os objetivos e os botões. Continua só de leitura: nada no jogo é destacado
nem feito; é o primeiro degrau do modo assistido do doc 10, seção 10.

**Contrato.** A janela de `GET /api/plugin/route` ganha, só no passo do cursor, `tasks`: uma linha por
tarefa, na ordem de execução (as herdadas das receitas primeiro), com `type`, `label` (o que fazer, já
em PT-BR), `until` (o que a termina, por extenso), `state` (`met`, `unmet` ou `unknown`), `reason` (por
que o servidor não sabe) e `recipe` (veio da receita de um objetivo). O servidor avalia o término
(doc 10, seção 10, "Como implementado"); o plugin nunca decide. Um planner anterior não manda o campo
e o painel fica como era.

```json
"tasks": [{"type": "travel", "label": "Ir até Lumbridge Castle", "until": "na área do local", "state": "met"},
          {"type": "talk", "label": "Falar com Hans: “Who are you?”", "until": "objetivos do passo", "state": "unmet"}]
```

**No painel.** "Tarefas", uma linha por tarefa: ✓ e "feito" em verde quando o servidor vê o término,
○ e "a fazer" quando não, e o ícone de "não sabemos" com "sem dado" quando o servidor não sabe (por
exemplo, sem "Send position" um `travel` fica sem dado, nunca "a fazer"). O tooltip diz o término, o
motivo e se a tarefa veio de uma receita.

**Verificação:** `TestAssistedModeTasks` e `TestAssistedModeWire` no servidor (este grava
`route-tasks.json` nos recursos de teste do plugin), `RouteWindowTest` no plugin lendo o mesmo arquivo
(156 testes JUnit) e o render sem tela do painel (`RoutePanelRenderTest`, variante `-tarefas`).
**Não conferido no jogo**: a lista com as tarefas reais, que só existem depois da gravação e da
confirmação do dono.
