# 12 — Conta autônoma: rota executável, quests pelo Quest Helper e supervisor local

**Estado: proposta (2026-11-24). Nada implementado.** As etapas e o estado de cada uma ficam no
Linear (seção 10). Continua a seção 11 do doc 11 ("rotas próprias e mais simples para contas novas,
do zero até um money making, de um jeito reproduzível"). As decisões estão na seção 11, e a
arquitetura e os dados, na seção 12.

## 1. A ideia

Uma conta **regular** (não ironman), nova, que se constrói sozinha num worker. Ela persegue as metas
de um modelo reutilizável, escrito desde o início no vocabulário do executor. Ela se mantém com
bonds comprados com GP de um catálogo de métodos de dinheiro. Ela chama um modelo de linguagem só
quando um passo trava.

O modelo de linguagem tem dois papéis, em momentos diferentes:

- **Offline**, o Claude Code escreve a rota e as receitas pelo fluxo de proposta, verificação e
  confirmação do doc 09, seção 6. Isso acontece uma vez, não por conta.
- **Em execução**, um modelo local (Ollama na RTX 4070 Super do zaros) destrava um passo de quest
  que o executor determinístico não resolveu. Ele age só pela lista fechada de comandos do doc 08.

## 2. Por que não seguir as rotas existentes

As rotas Oziris e BRUHsailer foram escritas para pessoas. Medido em 2026-11-24 nos sidecars de
`content/guides/*/annotations`:

| Rota | Passos | Com objetivo | Com tarefas tipadas |
|---|---|---|---|
| `oziris-main` | 578 | 347 | 0 |
| `bruhsailer-main` | 226 | 215 | 0 |

`content/recipes/` tem só o README. Os objetivos dizem o que é verdade depois do passo, não como
fazê-lo. O passo 1.1.47 da Oziris ("buy however many sardine you can fit in your inventory from the
fish shop in Witchaven") tem só `location:witchaven`. A loja, o item, a quantidade e a ordem ficam
no texto.

Traduzir 804 passos assim em tarefas é caro. Além disso, as duas rotas são de ironman: sem GE, com
desvios para conseguir itens que uma conta regular compra. Uma rota que nasce como metas
verificáveis evita a tradução.

## 3. A conta

- **Modo `regular`.** O doc 11 previu esse modo (seção 4.1, decisão W10). `accounts.mode` aceita
  `regular` desde a migração 0026, e o modo entra também nas variantes das receitas.
- **Sem rota.** `accounts.guide_id` aceita nulo desde a migração 0029. A conta sem rota segue o plano
  `custom` ativo (`accounts.active_plan_id`); sem plano escolhido, não segue nada, e o que lê a rota
  da própria conta (a janela do `now`, o próximo item do plano) responde 409 `account has no route`
  em vez de inventar um guia. Os fatos, os boards de cada guia e os cursores continuam calculados;
  nenhuma guia é a ativa, então a cascata de implícitos não roda (decisão W10 do doc 11).
- **Várias contas, cada uma descartável.** Uma conta até o ciclo estar provado, depois 2 a 3
  (decisão C5). Os modelos de metas, o catálogo de métodos e os overrides de quest (seção 5.6) valem
  para todas as contas, então o custo de escrevê-los se divide entre elas. Cada conta pode ser banida a
  qualquer momento (decisão 9 do doc 01). O desenho não pode depender de uma conta sobreviver.
- **Membership por bond.** A conta começa F2P. O primeiro bond custa alguns milhões de GP, e o preço
  varia. Contas F2P novas têm o GE e o trade limitados até cumprir horas jogadas, quest points e
  total level; os números e a fonte estão na seção 6.5.

## 4. A rota: metas da conta e um catálogo de métodos de dinheiro

A rota tem duas partes, e o planejador intercala as duas. A **construção** persegue as metas de
longo prazo da conta (seção 4.1). A **manutenção** paga a membership e os insumos com um catálogo de
métodos de dinheiro (seção 4.2). Não existe um método alvo único (decisão C1).

### 4.1 A construção: modelos de metas

As metas de uma conta vêm de um **modelo de metas** reutilizável, escrito uma vez e aplicado a cada
conta nova, com ajustes por conta (decisão C8). Um modelo é uma lista ordenada de **chaves de
objetivo** (doc 09), não de passos de texto. Por exemplo: `quest:cooks_assistant@finished`,
`skill:agility>=35`, `item:<id>`. Um modelo pode pedir uma quest sem listar os requisitos dela,
porque o planejador os resolve (seção 4.4).

A ordem das quests de um modelo vem do "Optimal quest guide" da OSRS Wiki, na variante para contas
regulares, filtrado pelas quests que o modelo pede (decisão C2). Os ajustes do dono ficam num arquivo
de patches, como `data/guide/route-patches.json` faz para a Oziris. A conta segue o modelo como um
plano `custom` do doc 10, seção 9, cujos itens já aceitam uma receita ou uma tarefa.

Os modelos de metas também respondem à decisão aberta 14 do doc 01 (metas prontas).

### 4.2 A manutenção: catálogo de métodos

A manutenção é uma restrição, não uma meta do modelo: **a membership não pode acabar**. Ela usa um
**catálogo de métodos de dinheiro**. Cada método do catálogo declara:

| Campo | O que é |
|---|---|
| Requisitos | Chaves de objetivo: níveis, quests, itens e membership |
| Execução | O plugin, a configuração dele e o lugar, no formato da tarefa `train` |
| Insumos e saídas | Item e quantidade por hora, para calcular o GP por hora pelos preços da seção 6 |
| Riscos | Os mesmos riscos das receitas (doc 10, E9) |
| GP por hora medido | O que a conta mediu rodando o método, pelo `rhythm`; substitui a estimativa quando existe |

Um método é **elegível** para uma conta quando todos os requisitos estão provados e o plugin dele
existe no perfil do cliente. A meta de GP escolhe, entre os elegíveis, o de maior GP por hora
líquido. A escolha é refeita quando os preços mudam ou quando a conta desbloqueia um método novo.

A manutenção passa por três etapas:

1. **F2P.** A conta cumpre os requisitos que liberam o GE (seção 3) e junta o primeiro bond com um
   método F2P do catálogo.
2. **Members.** A construção avança enquanto o banco tem GP para o próximo bond. Quando faltam menos
   de N dias de membership e o banco não tem esse GP, a meta de GP passa à frente da construção até
   o bond ser comprado.
3. **Folga.** Com GP para mais de um bond no banco, o tempo de jogo vai todo para a construção.

Quando uma skill da construção tem um método de treino com lucro no catálogo, a receita dessa skill o
prefere, e treinar também paga a membership. Cada conta se mantém sozinha: contas nunca trocam itens
nem GP entre si (decisão C7).

### 4.3 Receitas do modo regular

Cada chave da construção e a meta de GP têm uma receita em `content/recipes/` com uma variante `regular`:

| Chave | Tarefa da receita | Existe hoje |
|---|---|---|
| quest | `quest`: a preparação e o executor de passo da seção 5 | Delegação ao Quest Helper (E19), sem executor próprio |
| skill | `train`: um plugin com a configuração do método | Sim, nunca rodou em jogo |
| item | `ge_buy` (seção 6) | Sim, nunca rodou em jogo |
| lugar | `travel`, ou um teleporte comprado | `travel` sim; teleporte não (E14) |
| GP | `train` de um método de dinheiro, mais `ge_sell` das saídas | `ge_sell` sim, nunca rodou em jogo; o catálogo de métodos não |

### 4.4 Requisitos

O planejador insere uma receita só para um requisito provadamente não cumprido (doc 10, seção 9).
`data/wiki/quests.json` já traz os requisitos de cada quest. Se uma quest pede 35 de Agility e a
conta tem 30, o planejador põe a receita de Agility antes. A rota não precisa escrever esse passo.

## 5. Quests: o Quest Helper como mapa

O dono confirmou que a automação de quests do Quest Helper fica presa com facilidade. A proposta
mantém o Quest Helper como a fonte dos requisitos e do próximo passo e tira dele a execução. A
execução de uma quest é determinística por padrão, da preparação até o fim. O modelo de linguagem só
entra quando um passo trava (seção 5.7).

### 5.1 O que o Agent Server já dá

Fonte: `microbot/vendor/microbot/docs/AGENT_SERVER.md` e `QuestHelperHandler.java`, Microbot 2.6.52.

- `GET /quest-helper/status`: a quest selecionada, se ela terminou, o texto do passo atual
  (`stepText`) e a classe do passo (`stepClass`: `NpcStep`, `ObjectStep`, `ItemStep` e outras).
- `GET /dialogue`: se há diálogo aberto, a pergunta e as opções em texto. O `game_view` do agent não
  o inclui hoje.
- `GET /var-changes`: as últimas 500 mudanças de varbit e varp. O Chronicle não o usa hoje.

### 5.2 Preparação: requisitos antes do primeiro passo

Antes de começar uma quest, o planejador confere três tipos de requisito e insere uma receita para
cada um provadamente não cumprido:

| Requisito | Fonte | Receita inserida |
|---|---|---|
| Níveis de skill | `skill_reqs` de `data/wiki/quests.json` | `train` |
| Quests anteriores | `quest_reqs` de `data/wiki/quests.json` | a receita da quest anterior |
| Itens | Requisitos de item do Quest Helper, com id e quantidade | `ge_buy` (seção 6), ou o item é obtido durante a quest quando o helper o marca assim |

O catálogo da wiki traz os itens só como wikitext (`items_required_raw`). O Quest Helper tem os
mesmos itens estruturados, então ele é a fonte dos itens. Os itens recomendados (comida,
teleportes) entram no loadout quando a receita da quest os pedir.

Com os requisitos cumpridos, uma tarefa `bank` monta o inventário e o equipamento da quest. Só
então o executor de passo (seção 5.4) começa.

### 5.3 O que falta: requisitos e passo estruturados

O Agent Server devolve só o texto e a classe do passo. O objeto do passo no Quest Helper tem o alvo:
o id do NPC ou do objeto, o tile, os itens pedidos e as opções de diálogo que o helper destaca. Dois
endpoints novos no Chronicle Link leriam esses dados:

- `/bridge/quest-requirements`: os requisitos de item da quest selecionada, com id, quantidade e se
  o item pode ser obtido durante a quest;
- `/bridge/quest-step`: o alvo estruturado do passo ativo.

### 5.4 Executor de passo

Uma tarefa nova do Chronicle Link executa o passo estruturado:

- `NpcStep`: anda até o tile, fala com o NPC e escolhe as opções destacadas;
- `ObjectStep`: anda até o tile e interage com o objeto;
- `ItemStep`: pega o item do chão, ou o compra quando a receita permitir;
- qualquer outra classe vai direto para a escada da seção 5.7.

O progresso conta só quando o varbit da quest anda (`/var-changes`, e o fato `quest` do ingest), como
o `run_until` do doc 10.

### 5.5 Combate em quest

- A receita da quest declara o nível de combate recomendado com folga. O planejador insere o treino
  antes, como qualquer requisito.
- A receita declara um loadout de boss, com comida e equipamento, e a tarefa `bank` o monta.
- A luta usa um plugin de combate pelo `train`, com o NPC do passo e o limite de HP para comer.
- Numa conta regular, a morte é recuperável. O executor detecta a morte, recupera o gravestone e
  tenta de novo com um orçamento. Depois do orçamento, o passo vai para a escada.
- Safespots e troca de prayer entram como overrides daquele passo (seção 5.6).

### 5.6 Overrides por quest e passo

Quando o modelo local, o Claude ou o dono destrava um passo, a sequência de comandos que funcionou é
gravada como um override de (quest, passo). A gravação de demonstração do doc 10, seção 11, já
captura esses eventos. O override entra em `content/` só pela verificação e pela confirmação do dono
(regra 9 do CLAUDE.md). Na próxima conta, o executor usa o override e não chama ninguém.

### 5.7 Escada de escalonamento

Um passo está **travado** quando o varbit da quest não anda depois de N tentativas ou M minutos,
independentemente do que o Quest Helper mostre. Cada degrau custa mais que o anterior:

1. o override de (quest, passo), se existir;
2. o modelo local, com orçamento de ações (seção 7);
3. o Claude, com orçamento em tokens por passo;
4. o dono: o executor para, grava `run_events` com estado e screenshot e abre uma issue com a quest,
   o passo, o texto do helper e as últimas ações.

## 6. GE e dinheiro

Uma conta `regular` negocia no Grand Exchange por duas tarefas tipadas, `ge_buy` e `ge_sell`. Uma
conta ironman (`ironman`, `hcim`, `uim` ou `gim`) nunca as roda. O verificador de conteúdo só as
aceita numa variante de receita cujos modos são `[regular]`, nunca num passo de guia. A criação de
plano, `plan.Executable`, `plan.Resolve` e o `task_run` do agent recusam qualquer outro modo
(`execspec.Task.CheckMode`).

### 6.1 As tarefas

| Tarefa | Conteúdo | O que o servidor resolve (`execspec`) | Termina quando |
|---|---|---|---|
| `ge_buy` | `item`, `qty` | `geBuy: {item, itemName, qty, price}`. `qty` é o que falta, contado como na compra em loja, até o limite de 4 horas do GE. `price` é o último preço de compra imediata mais 5%, arredondado para cima. O inventário ao vivo precisa ter as moedas da oferta inteira | as posses da conta provam `item:<id>x<qty>` |
| `ge_sell` | `item` | `geSell: {item, itemName, qty, price, coinsAtLeast}`. `qty` é todo o item que o inventário ao vivo tem, numa forma só (noted ou não). `price` é o último preço de venda imediata menos 5%, arredondado para baixo. `coinsAtLeast` são as moedas do inventário naquela hora mais o mínimo que a venda paga | `sold`: um run da tarefa provou a venda (seção 6.2). O item só ausente do inventário não prova nada |

O `item` das duas tarefas é o id unnoted. O verificador de conteúdo recusa um id noted, porque o
slot do GE e os preços da wiki usam o id unnoted. A venda aceita o item nas duas formas no
inventário.

O preço vale só se a negociação por trás dele tem menos de 24 horas (`ge.MaxAge`). O mínimo que
uma venda paga é `qty × (price − imposto)`. O imposto do GE é 2% do preço de cada item, arredondado
para baixo e limitado a 5M por item ([OSRS Wiki, Grand
Exchange](https://oldschool.runescape.wiki/w/Grand_Exchange#Convenience_fee_and_item_sink)). Uma
oferta de venda enche pelo preço pedido ou melhor, e um item isento só aumenta o que entra, então o
mínimo vale para todos.

### 6.2 Execução

O agent delega as duas tarefas ao `/ge` do Agent Server (`GrandExchangeHandler` do cliente
fixado), como delega a quest ao Quest Helper (E19). O run:

1. confere que nenhum slot do GE já tem oferta do item e que nenhum outro slot tem oferta terminada
   e não coletada. Uma venda também recusa qualquer outra oferta no GE. O `/ge/collect` é o
   "Collect all" do GE e esvazia todos os slots terminados, então uma oferta de outro item poderia
   trazer moedas para o inventário e fazer a prova da venda passar sem a venda ter enchido;
2. manda `POST /ge/buy` ou `/ge/sell` com `{name, quantity, price}`. O Agent Server acha o item pelo
   nome exato, e o GE precisa estar aberto ou perto, então um `travel` antes leva a conta até lá;
3. acompanha `GET /ge` até o slot chegar a `BOUGHT` ou `SOLD`;
4. coleta no inventário (`/ge/collect` com `toBank: false`), para o inventário ao vivo mostrar o
   que chegou, e fecha o GE (`/ge/close`).

Uma oferta que não enche no prazo (10 minutos por padrão), o cancelamento do dono e qualquer falha
depois que o pedido da oferta saiu deixam o GE sem a oferta do run. Isso vale também quando o
`/ge/buy` ou `/ge/sell` responde erro: a oferta pode ter entrado mesmo assim (o `processOffer` falhou
depois de confirmar, o pedido passou dos 60 segundos ou o dono cancelou no meio). O run lê `GET /ge`,
aborta a oferta viva do item e coleta no inventário uma oferta já terminada. Só para quando o
`/ge/close` dá certo e o GE não tem oferta do item. O `/ge/close` pega a mesma trava do Agent Server
que a colocação da oferta, então um fechamento que dá certo prova que nenhuma colocação ainda roda lá.

O `/ge/abort` do cliente fixado chama `Rs2GrandExchange.abortOffer(name, true)`, que coleta todos os
slots terminados no **banco**. Depois de um abort, o que a oferta encheu e o que voltou dela (os
itens de uma venda, as moedas de uma compra) estão no banco, não no inventário. A prova de uma
compra continua valendo, porque as posses da conta contam o banco. A prova de uma venda não passa,
porque as moedas precisam estar no inventário.

O run terminar `done` quer dizer que a oferta encheu e foi coletada no inventário, nunca que a
tarefa está feita. Quem prova é o ingest. `GET /api/microbot/task` avalia a tarefa nos fatos da
conta. Uma compra termina quando as posses provam `item:<id>x<qty>`. Uma venda é provada quando o
inventário ao vivo, com baseline completa, não tem o item e tem pelo menos as moedas que o run
carrega (`RunTask.coinsAtLeast`, a regra `job.SaleGoal`, que diz o mesmo que `rules.SaleProven`).

Essa prova é relativa às moedas de antes da venda e some quando a conta gasta as moedas. Por isso a
venda é um job `ge_sell` (seção 13): o job tracker o leva a `succeeded` no momento em que vê a prova
e o job fica. O plano conta a venda como cumprida a partir daí (`plan.Input.SalesProven`, lido dos
jobs, o mais novo de cada tarefa que não foi cancelado):

| Inventário ao vivo | Venda provada por um run | `sold` |
|---|---|---|
| tem o item | qualquer | `unmet`: há o que vender |
| não tem o item | sim | `met` |
| não tem o item | não | `unknown`: nada a vender e nenhuma venda provada |
| não compartilhado ao vivo | sim | `met` |
| não compartilhado ao vivo | não | `unknown` |

O item no banco não é uma venda. Quando o item volta ao inventário, a tarefa fica `unmet` de novo.
O servidor apaga a prova anterior da tarefa antes de começar um novo run dela, então a venda nova só
fica cumprida pela prova dela, e o status do run (`GET /api/microbot/task`) sempre avalia o próprio
run, nunca a prova de um run anterior.

A prova só é gravada quando alguém lê `GET /api/microbot/task` enquanto as moedas ainda estão no
inventário; o `chronicle task run --wait` lê até a prova chegar. Se a conta gasta as moedas antes
dessa leitura, a venda fica `unknown` e o plano para nela. O Chronicle Link continua recusando
`ge_buy` e `ge_sell`; a seção `delegated` do `task-wire.json` fixa isso dos dois lados.

### 6.3 Preços

Os preços vêm da API de preços em tempo real da OSRS Wiki, versão 2
(`https://prices.runescape.wiki/api/v2/osrs`, rotas `/latest` e `/mapping`). As regras de uso estão
em [RuneScape:Real-time Prices](https://oldschool.runescape.wiki/w/RuneScape:Real-time_Prices): um
`User-Agent` descritivo, e uma chamada sem `id` em vez de uma por item. `internal/sources/wikiprices`
busca e normaliza. `internal/store/prices` guarda o resultado em `item_prices` (seção 12.3), com
fonte e hora, e um lado do preço nunca volta para uma negociação mais antiga. O `chronicle serve`
atualiza os preços ao subir e a cada `SYNC_PRICES_MINUTES` (60 por padrão; 0 desliga). Os preços
podem passar de 32 bits, então são `bigint`.

### 6.4 Membership

O fato `membership` (doc 06, adendo de 2026-11-24) traz os dias de membership restantes e se a
conta é membro. O cliente guarda os dias no varp 1780 (`VarPlayerID.ACCOUNT_CREDIT`) e a marca de
membro no varc `PLAYERMEMBER`; um membro pode ter 0 dias de crédito. Os dois ficam em 0 até o
servidor do jogo mandá-los, então o Chronicle Link só lê depois de dez ticks logado. A manutenção da
seção 4.2 lê este fato.

### 6.5 Restrições do GE para contas F2P novas

Fonte: OSRS Wiki, [Account, "New account trade
restrictions"](https://oldschool.runescape.wiki/w/Account#New_account_trade_restrictions), a mesma
seção que a página [Grand Exchange](https://oldschool.runescape.wiki/w/Grand_Exchange) inclui. Lida
em 2026-11-24. O catálogo de métodos confere estes requisitos:

- **Três condições, todas.** Uma conta F2P nova tem restrições de troca até passar de 20 horas de
  jogo logado, ter 10 quest points ou mais e chegar a total level 100. A wiki cita a notícia da
  Jagex "Easter Event", de 2 de abril de 2020.
- **Itens que a conta restrita não vende no GE.** São itens comuns de bots F2P: oak, willow e yew
  logs; raw shrimps, shrimps, raw anchovies, anchovies, raw lobster e lobster; clay e soft clay;
  copper, tin, iron, silver, gold, mithril, adamantite e runite ore, e coal; ashes; vial, vial of
  water e jug of water; fishing bait, feather, eye of newt e wine of Zamorak; air, water, earth,
  fire, mind e chaos runes; cowhide, leather e hard leather; red spiders' eggs; steel bar;
  redberries. Um método F2P que vende uma dessas saídas só é elegível depois das três condições.
- **Slots e itens.** Uma conta F2P tem 3 slots no GE e um membro tem 8; uma conta F2P só compra
  itens F2P. Vender não depende de membership.
- **Limite de compra.** Cada item tem um limite de compra a cada 4 horas ([Grand Exchange/Buying
  limits](https://oldschool.runescape.wiki/w/Grand_Exchange/Buying_limits)). O `/mapping` traz esse
  limite, `item_prices.buy_limit` o guarda e `ge_buy` não passa dele.
- **Bond.** Cada slot compra ou vende um bond só ([Bond](https://oldschool.runescape.wiki/w/Bond)).
  Um bond vale 14 dias de membership.

O Chronicle prova os quest points e o total level pelos hiscores e pelo ingest. As horas de jogo não
têm fonte no Chronicle, então o catálogo de métodos não consegue provar essa condição.

A meta de GP da seção 4.2 junta essas peças: um método de dinheiro (`train` com o plugin do método),
`ge_sell` das saídas e `ge_buy` do bond. A métrica da conta é quantos dias ela se manteve sem o
dono, com o GP por hora líquido de cada método.

## 7. O supervisor local

### 7.1 Escopo

O modelo local:

- destrava um passo de quest na escada da seção 5.7;
- escolhe entre métodos de treino ou de dinheiro equivalentes quando a receita deixa a escolha
  aberta;
- escreve o rascunho da issue de lacuna quando falta um plugin para um método que valeria a pena.

Ele não escreve plugins. Um plugin novo é trabalho do dono ou de uma sessão do Claude Code a partir
da issue.

### 7.2 Ferramentas

As ferramentas do modelo local são os comandos de jogo que a lista fechada do doc 08 já tem:
`game_view`, `walk`, `npc_interact`, `object_interact`, `inventory_use`, `item_use_on`,
`ground_pickup`, `dialogue_continue`, `dialogue_select`, `bank_open`, `bank_withdraw` e
`bank_deposit`. O agent valida os parâmetros de cada chamada nos dois lados, como hoje.
`game_view` passa a incluir o diálogo e o passo estruturado do Quest Helper. O modelo não tem acesso
ao teclado nem aos widgets, e não pode rodar scripts.

### 7.3 Contexto

A cada decisão, o contexto do modelo é refeito: o passo, o diálogo, as entidades próximas, o
inventário e as últimas K ações com o resultado de cada uma. O modelo não acumula histórico. Isso
cabe na VRAM de 12 GB e evita que ele perca o objetivo depois de dezenas de turnos.

### 7.4 Medição

A escolha do modelo é feita medindo, não estimando. Os mesmos passos travados vão para candidatos
locais (um denso de 14B e um MoE de cerca de 30B) e para o Haiku, pela mesma ferramenta. A medida é
a fração de passos destravados, as ações por passo e o tempo por decisão. Um modelo entra na escada
só se destravar mais passos do que ir direto para o dono.

### 7.5 O que muda no doc 10

O princípio 6 do doc 10 diz "O LLM é preparação, nunca execução", e a seção 11 diz que uma falha em
execução nunca chama LLM. A decisão 32 do doc 01 os registra. O dono revisou os dois em 2026-11-24
(decisão C3, registrada como decisão 41 do doc 01): em execução, um modelo de linguagem só age dentro de um passo
travado, só pela lista fechada de comandos, com orçamento, e o passo só termina quando o varbit
prova. Fora disso, continua valendo "só roda código verificado".

## 8. Sessões e supervisão

### 8.1 Sessões com pausas

Cada conta joga em sessões de duração variável, com pausas curtas e longas e um teto de horas por
dia (decisão C9). O Chronicle agenda as sessões, não o break handler do Microbot, porque a pausa
depende do plano:

- uma pausa só começa entre duas tarefas, nunca no meio de um passo de quest ou de uma luta;
- uma pausa longa faz logout, e a sessão seguinte faz login pelo worker (doc 11, seção 15.5);
- os parâmetros (duração da sessão, das pausas e o teto diário) são da conta, e cada um é sorteado
  dentro de uma faixa.

As rotinas recorrentes do doc 11, seção 16, já têm relógio e intervalo. O agendamento de sessões é
uma extensão delas.

### 8.2 Copiloto, depois autônomo

Uma conta começa no modo **copiloto** (decisão C10). Nesse modo, o planejador para e pede a
aprovação do dono antes de:

- começar uma quest que a conta ainda não fez;
- rodar um método de dinheiro ou de treino que ainda não foi aprovado para essa conta.

Uma aprovação vale para aquela conta e aquele item. O dono aprova pela web ou pela CLI. Quando o dono
muda a conta para o modo **autônomo**, o planejador para só na escada da seção 5.7 e nas travas de
segurança da seção 9. O dono revisa o histórico e as issues abertas depois.

## 9. Segurança e riscos

- **Ban.** Conta nova, grind repetitivo e bond comprado com GP é o perfil que a detecção da Jagex
  procura. O modelo local não muda o padrão do mouse e do teclado, que é dos plugins. Cada conta é
  descartável (seção 3), e o risco de várias contas no mesmo IP continua como W8 do doc 11.
- **Morte.** Numa conta regular, a perda é o tempo e o que não cabe no gravestone. O E9 continua
  valendo para contas `hcim`, e esta proposta não roda nelas.
- **Uma conta num lugar só** (doc 11, seção 6.2) e a pausa por intervenção humana (doc 08) continuam
  valendo para o executor de passo e para o modelo local.
- **Custo do Claude.** O degrau 3 da escada tem orçamento por passo e por dia. Sem orçamento
  restante, o passo vai para o dono.

## 10. Execução

As etapas, a ordem entre elas e o estado de cada uma ficam no Linear: projeto Chronicle, marco
"Conta autônoma" (FEL-94 a FEL-105). Este doc guarda a arquitetura, os dados e as decisões. Duas
regras de ordem são decisões e ficam aqui:

- o experimento com o Quest Helper (FEL-96) vem antes de qualquer código do executor de quest, porque
  mede se a seção 5 se sustenta;
- o modo `regular` com `train` num worker (FEL-97) vem antes de qualquer coisa que dependa do
  `train`, porque esse caminho nunca rodou em jogo.

## 11. Decisões

| # | Decisão | Estado |
|---|---|---|
| C1 | Um método de dinheiro alvo, ou vários | **Resolvida (2026-11-24, dono):** vários. Um catálogo de métodos, e o planejador escolhe entre os elegíveis (seção 4.2) |
| C2 | Ordem das quests da construção | Recomendação: o "Optimal quest guide" da OSRS Wiki para contas regulares, filtrado pelas quests do modelo de metas, com patches do dono |
| C3 | Um modelo de linguagem pode agir em execução? | **Resolvida (2026-11-24, dono):** só dentro de um passo travado, nos termos da seção 7.5 |
| C4 | Orçamento do Claude como degrau 3 da escada | Em aberto; decidir o valor depois da medição da seção 7.4 |
| C5 | Quantas contas | **Resolvida (2026-11-24, dono):** uma até o ciclo estar provado, depois 2 a 3 workers no saradomin. O risco de IP (W8 do doc 11) fica para quando houver mais de uma |
| C6 | Onde o modelo local roda | Recomendação: no zaros, servido pelo Ollama na rede interna para os workers do saradomin |
| C7 | Contas trocam itens ou GP entre si | **Resolvida (2026-11-24, dono):** nunca. Uma troca liga as contas para a detecção |
| C8 | De onde vêm as metas de cada conta | **Resolvida (2026-11-24, dono):** de modelos de metas reutilizáveis, com ajustes por conta (seção 4.1) |
| C9 | Como a conta distribui o tempo de jogo | **Resolvida (2026-11-24, dono):** sessões variáveis com pausas e teto diário, agendadas pelo Chronicle (seção 8.1) |
| C10 | Quanto o dono acompanha | **Resolvida (2026-11-24, dono):** copiloto no começo, autônomo depois, por conta (seção 8.2) |
| C11 | Plugins ligados e desligados, ou jobs | **Resolvida (2026-11-24, dono):** jobs com status e progresso, que executam e terminam, e um servidor movido a eventos (seção 13) |
| C12 | A primeira validação de ponta a ponta | **Resolvida (2026-11-24, dono):** a Tutorial Island da `acc_appendonly`, feita pelo agente de desenvolvimento com o plugin do Hub (seção 13.7) |

## 12. Arquitetura e dados

Esta seção junta as peças das seções 4 a 8 num desenho só. A regra que decide onde cada dado mora
vem do CLAUDE.md (regras 3 e 9): o que vale para todas as contas é **conteúdo**, fica no git e
passa por `chronicle content validate`; o que é de uma conta é **estado**, fica no Postgres.

### 12.1 Componentes

```
 content/ (git, verificado)          chronicle serve (saradomin)               worker da conta
 ─────────────────────────           ───────────────────────────               ───────────────
 goal-templates/  ─┐                 ┌─ condutor (novo) ──────────┐            chronicle agent
 methods/          │   loader        │   decide a próxima ação     │  task_run  │
 recipes/          ├──────────────►  │   de cada conta gerenciada  ├──────────► Chronicle Link
 quest-overrides/  │                 └──┬──────────┬───────────────┘            ├─ travel, bank, buy
 reference/       ─┘                    │          │                            ├─ ge_buy, ge_sell → /ge
                                        │          │ passo travado              ├─ train → plugin
                          plan.Next ◄───┘          ▼                            └─ quest → executor de
                          methods.Choose      escada (nova) ── Ollama (zaros)      passo (novo), lendo o
                          sessions.Due          │            └─ Claude              Quest Helper
                                                └─ issue / dono
 preços da wiki ──► sync de preços (novo) ──► Postgres ◄── ingest ◄──────────────── fatos do jogo
```

O condutor é a única peça que decide. O worker só executa uma tarefa tipada por vez e reporta. A
escada só age quando o condutor a chama, por um passo travado. As decisões puras (próxima ação,
escolha de método, próxima sessão) ficam em `internal/core` e são testadas por tabela (regra 4 do
CLAUDE.md); o condutor só lê o estado, chama essas funções e grava o resultado.

### 12.2 Conteúdo novo no git

Três tipos novos, cada um com schema em `content/schema/` e verificado pelo `content validate`. Os
ids, as chaves e os números dos exemplos são ilustrativos. A chave `membership` ainda não existe
entre as chaves de objetivo do doc 09; o fato que ela lê existe (seção 6.4).

**Modelo de metas** (`content/goal-templates/<id>.json`, seção 4.1). Uma lista ordenada de chaves de
objetivo. Os requisitos de cada chave não aparecem, porque o planejador os resolve.

```json
{
  "schema_version": 1,
  "kind": "goal_template",
  "id": "base-teleports",
  "title": "Base 50 e quests de teleporte",
  "modes": ["regular"],
  "goals": [
    "quest:waterfall_quest@finished",
    "skill:magic>=45",
    "quest:fairytale_ii__cure_a_queen@started"
  ],
  "source": { "quest_order": "osrs-wiki:optimal-quest-guide", "patches": "patches.json" }
}
```

**Método** (`content/methods/<id>.json`, seção 4.2). Um método de dinheiro, de treino ou os dois.
O catálogo de métodos substitui o plugin fixo nas receitas de skill: a receita de `skill:X>=N` deixa
de nomear um plugin e passa a pedir "o melhor método elegível que treina X".

```json
{
  "schema_version": 1,
  "kind": "method",
  "id": "molten-glass-edgeville",
  "modes": ["regular"],
  "requires": ["skill:crafting>=1", "membership"],
  "run": { "plugin": "<classe do Processor>", "config": [], "location": "location:edgeville" },
  "inputs":  [{ "item": "item:bucket_of_sand", "per_hour": 1400 }, { "item": "item:soda_ash", "per_hour": 1400 }],
  "outputs": [{ "item": "item:molten_glass", "per_hour": 1400 }],
  "xp_per_hour": { "skill:crafting": 28000 },
  "risks": []
}
```

Os números de `per_hour` são a estimativa inicial. O GP e o XP por hora medidos numa conta
substituem a estimativa para aquela conta (seção 12.4).

**Override de quest** (`content/quest-overrides/<quest>.json`, seção 5.6). O passo do Quest Helper
não tem id estável, então a chave do override é o valor do varbit da quest naquele passo, mais a
classe e o texto do passo como conferência. O corpo é uma sequência de comandos da lista fechada do
doc 08.

```json
{
  "schema_version": 1,
  "kind": "quest_override",
  "quest": "quest:waterfall_quest",
  "steps": [
    {
      "varbit_value": 4,
      "step_class": "ObjectStep",
      "step_text": "Use a rope on the rock.",
      "commands": [
        { "command": "walk", "params": { "x": 2512, "y": 3476, "plane": 0 } },
        { "command": "item_use_on", "params": { "item": 954, "object": 1996 } }
      ],
      "confirmed_by": "owner"
    }
  ]
}
```

As receitas existentes (`content/recipes/`) ganham variantes `regular`. A receita de quest declara
o nível de combate recomendado e o loadout (seção 5.5).

### 12.3 Estado novo no Postgres

| Tabela ou coluna | O que guarda | Seção |
|---|---|---|
| `accounts.mode` aceita `regular` (migração 0026) | O modo da conta | 3 |
| `accounts.guide_id` aceita nulo (migração 0029) | A conta sem rota, que segue o plano `custom` ativo | 3 |
| `accounts.supervision` (`copilot` ou `autonomous`) | Se o condutor pede aprovação | 8.2 |
| `plans.template_id` | O modelo de metas de onde o plano `custom` saiu; os itens são cópias das chaves do modelo, mais os ajustes da conta | 4.1 |
| `approvals (account_id, subject, approved_at)` | Uma aprovação por conta e por quest ou método (`quest:<ref>`, `method:<id>`) | 8.2 |
| `session_policies (account_id, …)` | As faixas de duração da sessão e das pausas e o teto diário | 8.1 |
| `play_sessions (account_id, started_at, ended_at, reason)` | As sessões que aconteceram; o teto diário é contado aqui | 8.1 |
| `conductor_state (account_id, state, task_key, run_id, escalation_id, since)` | Em que estado o condutor está para cada conta (seção 12.5) | 12.5 |
| `escalations (id, account_id, quest, varbit_value, step_class, step_text, status, …)` e `escalation_attempts (escalation_id, rung, model, actions, tokens, outcome)` | Cada passo travado e cada tentativa de cada degrau; é também a tabela da medição da seção 7.4 | 5.7, 7.4 |
| `item_prices (item_id, high, high_at, low, low_at, buy_limit, members, source, fetched_at)` | O último preço de cada item, da API da wiki (`/latest` e `/mapping`); não é de nenhuma conta. Migração 0028 | 6.3 |
| `jobs`, `events`, `event_cursors`, `routine_runs.job_id` (migração 0031) | Os jobs, o log de eventos e os cursores dos consumidores (seção 13); a venda no GE provada é um job `ge_sell` `succeeded`, que trocou a tabela `sale_proofs` da 0030 | 13, 6.2 |

O ingest tem a faceta `membership` (os dias restantes e a marca de membro, migração 0028) e ganha a
faceta `ge` (as ofertas abertas e o que está para coletar), pelo mesmo contrato v1. As duas entram
em `observations` e `player_state` como qualquer fato.

Nada disso guarda texto de guia, e nada disso é escrito pelo modelo de linguagem. A escada grava
tentativas em `escalations`; um override só vira conteúdo pela verificação e pela confirmação do dono.

### 12.4 Funções puras novas

| Pacote | Função | Entrada | Saída |
|---|---|---|---|
| `internal/core/methods` | `Eligible` e `Choose` | catálogo, fatos da conta, perfil do cliente, preços, medições da conta, propósito (GP, ou XP de uma skill) | os métodos elegíveis ordenados por GP por hora líquido, ou por XP por hora com o GP como desempate |
| `internal/core/upkeep` | `Due` | dias de membership, GP no banco, preço do bond, N | se a meta de GP passa à frente da construção (seção 4.2) |
| `internal/core/sessions` | `Next` | política da conta, sessões do dia, hora, semente | jogar até T, pausa curta até T ou pausa longa até T |
| `internal/core/conductor` | `Decide` | estado do condutor, resultado de `plan.Next`, `upkeep.Due`, `sessions.Next`, aprovações, o último evento do worker | a próxima ação e o próximo estado (seção 12.5) |
| `internal/core/questexec` | `Prepare` e `Stuck` | requisitos do catálogo e do Quest Helper, fatos; histórico de varbit e tentativas | as receitas a inserir antes da quest; se o passo está travado |

As medições por conta (GP e XP por hora de cada método) saem do `rhythm` e de `routine_runs.task`,
sem tabela própria.

### 12.5 O condutor

O condutor roda dentro do `chronicle serve`, como o reconciliador de workers, e trata cada conta
gerenciada como uma máquina de estados persistida em `conductor_state`. Um reinício do servidor
retoma do estado gravado.

| Estado | Sai quando | Vai para |
|---|---|---|
| `idle` | a sessão está aberta e existe próxima tarefa | `awaiting_approval` (copiloto, item não aprovado) ou `running` |
| `awaiting_approval` | o dono aprova | `running` |
| `running` | o servidor prova o término da tarefa | `idle` |
| `running` | a tarefa falha, ou o executor de passo declara o passo travado | `escalated` |
| `escalated` | um degrau destrava o passo | `running` |
| `escalated` | a escada esgota os degraus | `blocked` (o dono recebe a issue) |
| `idle` ou `running` | `sessions.Next` diz pausa e a tarefa atual chegou ao fim | `on_break` |
| `on_break` | a pausa acaba | `idle` |
| qualquer um | o dono pausa a conta ou intervém pelo VNC | `paused` |

Em `idle`, a próxima tarefa vem nesta ordem:

1. se `upkeep.Due` diz sim, a meta de GP: `methods.Choose` com propósito GP, e depois `ge_sell` das
   saídas e `ge_buy` do bond;
2. senão, `plan.Next` do plano da conta. Para uma receita de skill, `methods.Choose` com propósito
   XP escolhe o método. Para uma quest, `questexec.Prepare` insere as receitas dos requisitos que
   faltam.

O condutor despacha a tarefa pelo `task_run` que já existe, e o término continua sendo provado pelo
servidor (`run_until`, doc 10). O condutor nunca marca nada como feito. A seção 13 revê esta seção:
o condutor despacha jobs e muda de estado a cada evento da conta, e o término vem do job tracker.

### 12.6 Uma quest de ponta a ponta

1. `plan.Next` devolve `quest:waterfall_quest@finished`. A conta está em copiloto e a quest não foi
   aprovada, então o condutor vai para `awaiting_approval`.
2. O dono aprova. `questexec.Prepare` lê os níveis e as quests anteriores do catálogo e os itens do
   Quest Helper (`/bridge/quest-requirements`). Falta uma corda: o condutor despacha `ge_buy`, depois
   `bank` com o loadout.
3. O condutor despacha a tarefa `quest`. O executor de passo do Chronicle Link lê
   `/bridge/quest-step`, executa `NpcStep`, `ObjectStep` e `ItemStep` e reporta cada mudança de
   varbit pelo ingest.
4. No varbit 4, o passo não anda depois de N tentativas. O executor para e reporta o passo travado.
   O condutor abre uma linha em `escalations` e vai para `escalated`.
5. Não existe override. O modelo local recebe o passo, o diálogo e as entidades próximas e manda
   comandos da lista fechada, até o orçamento. O varbit anda para 5: a tentativa é gravada como
   destravada, e a sequência de comandos vira uma proposta de override para o dono confirmar.
6. O condutor volta para `running`, e o executor continua. O servidor prova
   `quest:waterfall_quest@finished` e o condutor volta para `idle`.

## 13. Jobs e eventos

O dono decidiu em 2026-11-24 (C11) que a execução se organiza em **jobs**: unidades com status e
progresso, que executam e terminam, e não plugins ligados e desligados. Decidiu também que as peças
do servidor reagem a **eventos** em vez de consultar umas às outras. Esta seção é o desenho; as
seções 6.2, 12.3 e 12.5 continuam valendo onde esta não as revê.

### 13.1 O que já existe e o que falta

O Chronicle já tem metade disso com outros nomes:

- um **run** (`routine_runs`, doc 08) já tem estado (`running`, `done`, `failed`, `canceled`), início,
  fim, trilha (`run_events`) e resultado; para o `train` e a `quest`, o agent liga o plugin no começo
  e desliga no fim, então o plugin já é um detalhe do run;
- o fim de um run com `until` já era provado pelo servidor a partir do ingest (o antigo `internal/rununtil`, agora o job tracker);
- o ingest já é append-only (`observations`), e `player_state` é a projeção dele;
- `internal/events` já entrega sinais de invalidação para a web (`/api/events`).

Falta:

1. **a intenção separada da tentativa.** "Sair da Tutorial Island" ou "Mining 15" é um job; cada vez
   que o agent tenta, é um run. Um job travado ganha outro run e continua o mesmo job;
2. **progresso medido**, tirado dos fatos, que também diz quando um job travou;
3. **eventos duráveis no servidor.** O antigo `rununtil` acompanhava os fatos por conta própria, e a prova
   de uma venda só é gravada quando alguém lê o status do run (FEL-123).

### 13.2 Job

Um job é uma linha em `jobs`. Ele tem:

| Campo | O que é |
|---|---|
| `kind` | `plugin`, `train`, `quest`, `travel`, `bank`, `buy`, `ge_buy` ou `ge_sell`. `plugin` é o novo: liga um plugin do perfil do cliente (do Hub ou nosso) com a configuração dada e o desliga quando o job termina |
| `spec` | a tarefa tipada do `execspec` que o agent executa, como hoje |
| `goal` | a regra que prova o fim, avaliada pelo motor de regras sobre os fatos (doc 09). Exemplo: `{"type": "varp", "id": 281, "min": 1000}` para a Tutorial Island, `skill:mining>=15` para um treino |
| `metric` | o número que mede o progresso e a meta dele: uma varp ou varbit, o XP até o nível, a contagem de um item, o varbit de uma quest |
| `progress` | `{from, current, target, at}`: o valor da métrica quando o job começou, o último observado, a meta e quando mudou |
| `stall_after` | quanto tempo sem progresso conta como travado; cada `kind` tem um padrão |
| `state` | a máquina de estados da seção 13.3 |
| `source` | quem criou o job: o condutor, o dono ou um teste |

O run continua como está, com uma coluna nova: `routine_runs.job_id`. Um job tem zero ou mais runs, e
no máximo um ativo.

### 13.3 Estados do job

| Estado | Sai quando | Vai para |
|---|---|---|
| `queued` | o condutor ou o dono despacha o job | `running` (um run começa) |
| `running` | a regra do `goal` vira `met` num fato `live` | `succeeded` |
| `running` | a métrica não muda por `stall_after` | `stalled` |
| `running` | o run termina sem o `goal` provado | `stalled` |
| `running` | o run de um `ge_sell` termina `failed` ou `canceled` | `failed` |
| `stalled` | quem trata o travamento começa outro run | `running` |
| `stalled` | a métrica muda com um run do job ainda ativo (o travamento foi o prazo, e o run continuou) | `running` |
| `stalled` | a escada esgota os degraus (seção 5.7) | `failed` |
| qualquer um ativo | o dono cancela | `canceled` |

Só o job tracker (seção 13.5) leva um job a `succeeded`, e só a partir de um fato do ingest. O fim de
um run (`done`) diz só que o agent parou. Isso é a regra "término provado pelo ingest" do doc 10,
agora garantida pela estrutura.

### 13.4 Eventos

Os eventos ficam numa tabela append-only:

```
events        id bigserial PK, account_id → accounts, type, subject (job, run ou sessão),
              payload jsonb, at
event_cursors consumer PK, last_id
```

- **Gravação.** Cada evento é gravado na mesma transação da mudança que o causa: a projeção de um
  fato, o registro de um run reportado pelo agent, a transição de um job.
- **Entrega.** Depois do commit, o hub em processo (`internal/events`) acorda os consumidores, como já
  acorda a web. Um só `chronicle serve` é dono do banco, então não entra broker nem `LISTEN/NOTIFY`.
- **Retomada.** Cada consumidor guarda o último id que tratou em `event_cursors`. Depois de um
  reinício, ele lê a partir dali. Os consumidores são idempotentes, porque um evento pode ser
  entregue de novo depois de uma queda entre o efeito e o cursor.
- **Fonte da verdade.** Um evento diz "mudou"; quem consome lê o estado (fatos, job, run). Para os
  fatos, `observations` e `player_state` continuam sendo a fonte, como diz o `internal/events`.

Os tipos:

| Tipo | Quem grava | Payload |
|---|---|---|
| `fact.changed` | a projeção do ingest | as facetas e chaves que mudaram |
| `run.started`, `run.ended` | o registro dos runs (`internal/routinesync`) | o run, o job e o estado final |
| `job.queued`, `job.progressed`, `job.stalled`, `job.succeeded`, `job.failed`, `job.canceled` | o job tracker e quem despacha | o job, o progresso e o motivo |
| `session.started`, `session.break`, `session.ended` | o agendador de sessões (seção 8.1) | a sessão |
| `membership.low` | o job tracker, a partir de `fact.changed` da faceta `membership` | os dias restantes |

O travamento por tempo não tem fato que o dispare. O job tracker guarda o prazo de cada job em
`running` e grava `job.stalled` quando o prazo vence sem `job.progressed`.

### 13.5 Consumidores

| Consumidor | Lê | Faz |
|---|---|---|
| **job tracker** | `fact.changed`, `run.ended` | Avalia `goal` e `metric` dos jobs ativos da conta, grava o progresso, `job.succeeded` e `job.stalled`. Absorve o `internal/rununtil`: o `run_until` ao agent sai daqui. Absorve a prova da venda: um `ge_sell` é `succeeded` quando o item saiu do inventário live e as moedas do run chegaram, e o plano lê o job, não mais `sale_proofs` (FEL-123) |
| **condutor** (seção 12.5) | `job.succeeded`, `job.failed`, `session.*`, `membership.low`, aprovações | Passa a ser movido por eventos: em vez de consultar o estado num laço, decide a cada evento da conta e despacha o próximo job |
| **escada** (seção 5.7) | `job.stalled` | Tenta destravar e começa outro run, ou leva o job a `failed` e abre a issue |
| **web** | todos, por conta | Estende o SSE de `/api/events` com os eventos de job, para a Frota mostrar o progresso ao vivo |

### 13.6 O que muda nos dados

| Tabela ou coluna | O que guarda |
|---|---|
| `jobs (id, account_id, kind, spec, goal, metric, progress, stall_after, state, reason, source, created_at, started_at, finished_at)` | os jobs (seção 13.2) |
| `routine_runs.job_id` | o job de cada run, nulo para os runs que o dono liga à mão |
| `events`, `event_cursors` | o log e os cursores (seção 13.4) |
| `conductor_state.job_id` | substitui `task_key` e `run_id` da seção 12.3 |

`sale_proofs` (migração 0030) foi trocada pelo job `succeeded` na migração 0031 (seção 13.8): a
tabela cai e as provas que ela ainda tinha viram jobs `ge_sell` já cumpridos. A 0031 gravou nesses
jobs uma meta provisória que o código não lê; a listagem de jobs os pula, e o plano continua contando
a venda pela `task_key`.

### 13.8 Como ficou

O que a implementação decidiu onde as seções acima não diziam (`internal/core/job`,
`internal/store/jobs`, `internal/store/eventlog`, `internal/jobtracker`):

- **Meta (`goal`).** Tem três formas em JSON: uma chave de objetivo (`"skill:mining>=15"`), uma regra
  do motor (`{"type": "varp", "id": 281, "min": 1000}`) ou `{"task": {...}}`, a tarefa tipada cuja
  terminação o painel avalia (posição numa área, loadout pronto). A venda no GE é a regra
  `não item_owned(item, inventory) e item_owned(moedas >= coinsAtLeast, inventory)`, que prova só com
  o inventário ao vivo e baseline completa. `succeeded` exige um `job.Proof`, que só uma avaliação
  `met` sem fato restaurado (`stale`) produz; nenhum outro caminho do código leva um job a esse estado.
- **Métrica.** `varp`, `varbit` ou `skill_xp`, tirada da meta quando o pedido não a traz: a varp ou
  varbit com `min`/`eq`, ou o XP do nível de uma chave de skill. Um job sem métrica (quest, venda,
  deslocamento) só trava quando o run termina sem a meta provada, nunca por prazo.
- **Estados.** Além da tabela da seção 13.3, `stalled` vai para `succeeded`: um run que termina antes
  de o ingest mostrar a meta trava o job, e o fato que a prova pode chegar depois. `running` vai para
  `failed` quando o run de um `ge_sell` termina `failed` ou `canceled`. Um `stalled` sem run ativo
  cujo último run terminou há mais de 13 h (o `wait_until` mais longo, com folga) vai para `failed`
  ("not proven within 13h0m0s of its last run"), no tick de segurança, e deixa de ser avaliado.
- **A venda é do run.** Um `ge_sell` só vira `succeeded` com o último run do job `running`, `waiting`
  ou `done`: prova no fim do run, ou depois enquanto aquele for o último run. Um run `failed` ou
  `canceled` leva o job a `failed` e nada que o inventário mostre depois o prova (a venda abortada, o
  vidro no banco, moedas sacadas para outra coisa).
- **Eventos.** A transição para `running` grava `job.started` (a seção 13.4 não nomeava esse evento).
  O job tracker lê também `run.started`, que é o que tira o job de `queued` ou de `stalled`. Um
  `stalled` com run ativo também volta a `running` quando a métrica muda, com `job.started` e o novo
  progresso (decisão do dono de 2026-11-25, FEL-133). Sem run ativo, a mudança não o move: ele espera
  outro run ou a prova da meta.
  Cada evento do job ou de um run do job leva `events.job_id` (migração 0032), indexado, que é por onde o detalhe
  `GET /api/jobs/{id}` os acha; os `fact.changed` com mais de 7 dias que todos os cursores passaram
  são apagados pelo job tracker (no máximo uma vez por hora).
  `fact.changed` lista as chaves que mudaram por faceta (até 100 por faceta) e não sai para a posição e
  os timers, que mudam a cada push; o job tracker avalia as metas de posição a cada 30 s e quando o
  run termina. Depois do commit, quem grava acorda os consumidores pelo hub (`events.TypeLog`,
  `SubscribeAll`), que nunca chega ao SSE da web.
- **Retomada.** O cursor `job_tracker` anda depois de cada lote, e os efeitos do lote vêm antes dele;
  um evento cujo tratamento falhou não é ultrapassado, e o próximo passe o lê de novo. Um evento
  entregue de novo não muda nada, porque cada tratamento lê o estado de novo.
- **Ordem do log.** O `id` vem da sequência na ordem dos `INSERT`, não na ordem dos commits, então um
  leitor que anda por `id` poderia passar por um evento que commita tarde. Quem grava segura o lock
  consultivo compartilhado do log (`pg_advisory_xact_lock_shared`) do `INSERT` até o commit, e o
  leitor (`eventlog.After`) pega o mesmo lock exclusivo, com `try`, antes de ler: nunca lê com um
  evento ainda sem commit. Um lock que os escritores não soltam em 1 s é `ErrBusy`, e o passe seguinte
  tenta de novo. O `xmin` do snapshot não serve: ele é do cluster inteiro, e uma transação lenta de
  outro banco travaria o tracker. O tick de segurança também começa o job `queued` cujo run está
  ativo, caso o `run.started` tenha passado sem efeito. O prazo de
  travamento fica em `jobs.deadline_at` (a última mudança da métrica mais `stall_after`), então
  sobrevive a um reinício; o job tracker dorme até o menor prazo.
- **`run_until`.** Quando um job fica `succeeded` com um run ainda ativo e a meta é uma chave de
  objetivo, o job tracker manda `run_until` ao agent, uma vez por run e auditado com o ator
  `job_tracker`. Um run que o dono começa por "Fazer agora" com `until`, ou que um gatilho ou uma
  recorrente começa, vira um job `plugin` adotado quando o `routinesync` grava o run (fonte `owner` ou
  `routine`), para o mesmo caminho servir a todos.
- **Run e job.** A tarefa tipada leva `jobId` até o agent (`TaskRunParams`, `RunTask`) e volta no
  relatório do run, que `routines.RecordRun` liga a `routine_runs.job_id`. O servidor só manda o
  campo a um agent que declara a feature `job_id` no hello: o `decodeParams` do agent recusa campo
  desconhecido. Para um agent sem a feature, o `RecordRun` liga o run ao job `queued` que o servidor
  criou para a mesma tarefa (conta, plano, `task_key` e tipo) e que nenhum run tem ainda, e então não
  adota um segundo job `plugin` para o run. Não há restrição no banco de um run ativo por job: o
  servidor cria um job por run. Um `task_run` que falha por erro que pode ter começado o run (prazo,
  link perdido) não cancela o job; só a recusa do agent, ou a falta de agent, o cancela. Os jobs do
  `POST /api/jobs` nunca levam plano nem `task_key`: só o run de tarefa do servidor os escreve, e o
  plano lê a prova da venda por eles. Os runs que já estavam em andamento no deploy da migração 0031
  ficam sem `job_id`, uma vez só.
- **Agent sem conta declarada.** Um agent anexado sem conta no hello tem o run tomado pela palavra do
  próprio run: a conta do `until` (`wait_until`) ou a do job que a tarefa leva, e só quando o
  `Link.Route` dessa conta devolve esse mesmo agent (o `route` entrega a conta pedida a um único agent
  sem conta). Sem isso, o `run_until` e a prova da venda se perderiam para esse agent.
- **`membership.low`.** Grava-se na primeira mudança que acha a assinatura em 7 dias ou menos, e não
  outra vez em 8 dias: as mudanças seguintes e a reentrega não repetem. Uma renovação e uma nova
  queda depois de 8 dias gravam de novo.
- **Colunas a mais em `jobs`.** `plan_id` e `task_key` (o plano lê os `ge_sell` por elas), `deadline_at`
  e a fonte `routine`.
- **Job `plugin`.** A `spec` é a tarefa `plugin` do `execspec` (`{"type":"plugin","plugin":{"plugin":<classe>,
  "name":...,"config":[...],"until":...}}`). O `until` é a meta como o agent espera por ela: a chave de
  objetivo ou um rótulo de var (`varp:281>=1000`), que só uma meta `varp` ou `varbit` com `min` produz
  (`Goal.Until`); o servidor o preenche a partir da meta ao criar o job e recusa um `until` que não seja
  o dela. O `clientcfg` restringe os plugins que nomeia (`KnownPlugins`): o `TutorialIslandPlugin` só
  recebe os cinco `toggle*` do grupo `MicroTutIsland`, com `true` ou `false`; nenhuma chave nem o script
  dele escolhe o modo ironman, então nada é fixado (`PinnedSettings`). Um plugin que o registro não nomeia
  aceita qualquer configuração válida.
- **Despacho.** `POST /api/jobs/{id}/run` (`chronicle jobs run <id> --yes`) põe um job `plugin` `queued` ou
  `stalled` na conta: confere que o agent declara as features `plugin_task` e `job_id` e que o cliente tem
  o plugin, e manda `task_run` com o `jobId`, sem chave nem plano. Uma recusa ou um agent ausente deixa o
  job como estava. A varp 281 entra no manifest por `ingest.JobVarps`. O agent liga o plugin como num
  treino (`driveDelegated`), e o job tracker manda `run_until` com o rótulo quando prova a meta; o
  progresso final do job é a última leitura da métrica, mesmo quando a prova chega entre duas leituras.
- **Diálogo no ponto seguro.** O ponto seguro de um job `plugin` não espera um diálogo que só tem
  "Click here to continue": o agent o avança (`/dialogue/continue` do Agent Server, até 20 linhas) e
  anota "avançando o diálogo" na trilha. Um diálogo com opções continua segurando o run, porque é uma
  escolha que o plugin não fez. Os outros runs (gatilho, treino, quest, tarefa) esperam qualquer
  diálogo, como antes. Decisão do dono de 2026-11-25 (FEL-130), depois de a caixa do tutorial segurar
  dois runs na validação da Tutorial Island.
- **Fora do escopo.** O condutor movido por eventos (FEL-126), a escada e o SSE da web com os eventos de
  job.

### 13.7 A validação: a Tutorial Island da AppendOnly

O dono escolheu em 2026-11-24 (C12) a Tutorial Island como a primeira validação de ponta a ponta,
feita pelo agente de desenvolvimento, com a conta `acc_appendonly`. A ilha prova o que um treino não
prova sozinho: um plugin do Hub dirigido por um job, uma métrica que não é XP e um fim sem nível.

- **Job:** `kind: plugin`, o `TutorialIslandPlugin` do Microbot Hub pelo perfil do cliente,
  `goal` `{"type": "varp", "id": 281, "min": 1000}`, `metric` a varp 281.
- **Ingest:** a varp 281 entra no manifest do servidor. O Chronicle Link já manda as varps que o
  manifest pede, sem mudar o plugin.
- **O que é observado:**
  - os estados do job e os eventos de cada transição;
  - o progresso da varp 281 ao vivo na web;
  - o fim provado pelo ingest e o plugin desligado depois;
  - um travamento provocado, por exemplo pausando o plugin, que leva o job a `stalled`.
- **Retomada:** a ilha não se repete, mas o job começa da varp atual. Um run interrompido no meio
  não perde a validação, porque o próximo continua de onde a conta está.
- **Login:** só com a conta deslogada do cliente do dono, para os dois não se derrubarem.
