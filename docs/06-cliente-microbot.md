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
| Collection log | `collectionLogSlots` via enums/structs do chisel, como o WikiSync faz | login e ao abrir o log |
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

Decisão do dono: um só plugin do Chronicle no microbot-plugins. A ponte de controle do doc 08 (menus
de NPC, objeto e equipamento, orações, feitiços e as ações tipadas deles) e o export das opções de
config saíram do Dev Bootstrap para o `fm/ironmanplanner` 1.3.0. Eles só sobem no cliente de dev do
harness (`-Dmicrobot.dev.harness=true`, o do `mb-client`) e só respondem com o token do Agent
Server; no cliente do dia a dia o plugin continua só lendo e nada disso existe. O tracking (o
escopo abaixo) segue somente leitura em qualquer cliente.

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
- Device flow de `oidc` (item 15): só o ponto de extensão documentado na interface, sem implementação.

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
