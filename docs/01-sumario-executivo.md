# Sumário executivo e registro de decisões

Data: 2026-11-03; atualizado em 2026-11-04. É o **registro único das decisões** do projeto: cada
decisão aparece uma vez aqui, com estado e onde está detalhada. Os docs 03 e 09 apontam para esta lista
em vez de manter listas próprias. O roadmap, o estado e os próximos passos ficam no Linear, no
projeto Chronicle (<https://linear.app/felipemalacarne/project/chronicle-1f382104f1b7>). Os docs
guardam a arquitetura e as decisões.

## O que já existe no repositório

| Ativo | Onde | Estado |
|---|---|---|
| Guia Oziris completo, 7 seções, 576 steps, com metadados (quest, skillGoal, items, location, layer enhanced) | `data/guide/*.json` | extraído do payload do ironman.guide |
| Guia normalizado com regras `complete_when` e prerequisites | `data/guide/normalized/guide.json`, `stats.json` | gerado por `tools/normalize_guide.py`, validado sem erros |
| Ordem de quests do site (215 quests, 6 fases, 60 marcos) | `data/guide/normalized/quest-order.json` | gerado |
| 30 páginas companheiras do site em Markdown (gear, bosses, hcim, uim, gim, money, afk, 18 artigos) | `data/raw/ironman-guide/pages/` | extraído |
| 225 quests do wiki com requisitos de skill e quest, QP, série, XP de recompensa, aliases para WikiSync | `data/wiki/quests.json` | gerado por `data/wiki/scripts/build_quests.py` |
| Todos os itens do jogo com página na wiki: 14.870 ids com nome, forma noteada, slot, família e variante | `data/wiki/items.json` | gerado por `tools/extract_wiki_items.py` |
| 492 tasks de achievement diary por área e tier | `data/wiki/diary_tasks.json` | gerado |
| 124 respostas reais de API (hiscores, WOM, WikiSync, Bucket, chisel, TempleOSRS, RuneLite) | `data/samples/` | salvas |
| DOCX pinados do BRUHsailer com checksums | `data/sources/bruhsailer/` | entrada de `content import-bruhsailer` |


## Números que importam

Cobertura de auto-tracking medida sobre os 576 steps (fonte: doc 05):

| Via | Steps | % |
|---|---|---|
| Hiscores (skill_level) | 50 | 9% |
| Plugin próprio (quest_state, diary_tier) | 159 | 28% |
| Manual | 367 | 64% |

A regra de cascata (concluir um step posterior implica os anteriores da mesma seção, estado `implied`)
eleva a cobertura prática para perto de 90%, mas precisa ser visualmente distinta e confirmável. O doc 03
traz estimativas um pouco mais otimistas; os números medidos acima prevalecem.

No BRUHsailer a cobertura automática era **zero** (226 passos longos e nenhuma regra), o que motivou o
modelo de objetivos e a extração por agente do doc 09. Desde 2026-11-05 os 226 passos estão anotados
(308 objetivos de quest, 68 de skill, 31 de diary, 385 de item); 28 passos ficaram sem objetivo.

Quests são o sinal dominante e dependem do cliente. Desde o plugin 1.1.0 (item 7, 2026-11-05) o plugin
envia o estado de todas as quests, os varbits de diary e o varp 101 (quest points) na faceta `data.quest`
e nos vars genéricos do contrato v1. Sem plugin próprio o projeto ficaria refém do WikiSync, cuja
API o wiki pede explicitamente para não usar em projetos de terceiros.

## Registro de decisões

| # | Decisão | Estado | Onde |
|---|---|---|---|
| 1 | Go como núcleo: um binário com `serve`, CLI e servidor MCP; Postgres (pgx/sqlc/goose); SPA Svelte embutida | resolvida (2026-11-03) | doc 04, seções 1 e 2 |
| 2 | Plugin próprio no Microbot, somente leitura, no MVP; WikiSync fora | resolvida (2026-11-03) | doc 06; doc 03 F7/F8 |
| 3 | Cascata `implied` no MVP, só dentro da seção e só na rota ativa, sempre confirmável | resolvida (2026-11-04) | doc 09, 9.1 |
| 4 | Local-first no `zaros` com `AUTH_MODE=dev`; `saradomin` com Zitadel depois, mesma imagem | resolvida (2026-11-04) | doc 04, seção 8; doc 07 |
| 5 | BRUHsailer como rota independente, nunca camada do Oziris; comparação por marcos compartilhados | resolvida (2026-11-04) | doc 05, seção 11; CLAUDE.md regra 7 |
| 6 | Objetivo como unidade rastreável, Ref canônico, progresso compartilhado entre rotas, marcas e cursor por guia | resolvida (2026-11-04) | doc 09, seções 2 a 5 |
| 7 | Extração dos textos por agente no Claude Code com MCP stdio local e skill, consulta interativa ao dono, resultado commitado e verificado | resolvida (2026-11-04) | doc 09, seção 6 |
| 8 | `unlock` e `location` entram já nesta revisão, povoados pela extração | resolvida (2026-11-04) | doc 09, 3.1 |
| 9 | Controle do Microbot pelo planner (agent no zaros, VNC, plugins, rotinas), módulo separado e desligado por padrão, risco aceito | resolvida (2026-11-04) | doc 08 |
| 10 | Revisão crítica do doc 09 (D1 a D10): faceta `data.quest` no contrato, eventos por guia, anotações como sidecar, item não exato sem chave, `content_steps` mínimo, ids posicionais do BRUHsailer, ordem da sequência mantida, catálogo de itens do bucket, doc 04 reescrito, `manual_ok` fora | resolvida (2026-11-04) | doc 09, 9.3 |
| 11 | Grafo visual na v1 junto com a lista em árvore, ou só a lista primeiro | resolvida (2026-11-08): os dois, como o canvas v6 desenhou: colunas por profundidade em 1440 (V6Grafo1440) e árvore com recuo em 640 (V6Grafo640), em HTML, sem Cytoscape/ELK | doc 03, F10; doc 04, seção 5 |
| 12 | Modos iniciais: ironman + HCIM; UIM e GIM ficam fora até alguém pedir | **aberta** (recomendação: sim) | doc 03, F11 |
| 13 | Mobile como PWA instalável | **aberta** (recomendação: sim) | doc 03, seção 4 |
| 14 | Metas pré-definidas iniciais: títulos das 7 seções, Barrows gloves, Fire cape, Quest cape, Bowfa | **aberta** | doc 03, F9 |
| 15 | Estimativas de tempo na v1 como "could" com tabela de rates à mão, ou depois do tracking confiável | **aberta** | doc 03, F14 |
| 16 | Repositório público ou privado; público exige resolver a licença dos textos dos guias antes | resolvida (2026-11-12): **público**, por decisão do dono, assumindo o risco de redistribuir os textos dos guias (Oziris e BRUHsailer, inclusive os `.docx` em `data/sources/bruhsailer` e o histórico do git), que não declaram licença; a atribuição continua em todas as telas (regra 6) e nos README dos guias. Motivo imediato: o nix-config referencia o flake por `github:` sem credencial (doc 07, seção 3.4) | doc 05, seção 8; doc 07, seção 3.4 |
| 17 | Nome da conta ironman para testar o sync real desde a primeira sessão | resolvida na prática (2026-11-04): o sync do hiscores roda na conta do dono desde o item 10 do doc 04 | doc 04, seção 6.1 |
| 18 | Catálogo de itens com o jogo inteiro (toda página de item da wiki), não só o que os guias citam; por nome vale o item da página com aquele título | resolvida (2026-11-04) | doc 09, seção 7 (substitui o escopo de D8) |
| 19 | Checagem de ordem da extração conta o XP fixo de recompensa das quests como fonte de nível | resolvida (2026-11-04) | doc 09, seções 6.8 e 7 |
| 20 | Entradas de `unlock`/`location` corrigidas só pela tool `revise_entity`, com confirmação do dono | resolvida (2026-11-04) | doc 09, seção 6.8 |
| 21 | Progresso, web e hiscores (itens 8 a 10 da sequência) andam em paralelo com a extração completa (item 6), numa sessão e branch próprias, para o dono usar e testar o que já foi extraído | resolvida (2026-11-04) | doc 04, seção 9 (ajusta D7 do doc 09) |
| 22 | Passo bloqueado não segura o cursor: `requires` em `unmet` do passo ou requisito de catálogo da quest que ele conclui (fora `during_quest`, `boostable` e o que o passo conclui); `unknown` não bloqueia; `pinned_open` e pin seguram; passos flutuantes sem marcador no conteúdo | resolvida (2026-11-04) | doc 09, seção 3.5; doc 03 |
| 23 | Chaves de item não exatas: `item:family/<slug>xN` e `item:any(<id>x<n>\|…)` com quantidade por alternativa e forma canônica; graceful pela família padrão (recolor não conta) | resolvida (2026-11-04) | doc 09, seção 3.2 (revê D4) |
| 24 | Plugin lê qualquer varbit/varp que o manifest pedir (unlocks, CA e afins viram trabalho só de servidor e catálogo); 1.1.0 com quests, diaries e qp; 1.2.0 com painel da rota, posição/mundo (opt-in), collection log e CA | resolvida (2026-11-04) | doc 06; doc 04, seção 9 |
| 25 | Tempo real na web por SSE (`GET /api/events`) com refetch: o evento só avisa que algo mudou, sem segunda fonte da verdade; hub em memória (um processo), sem WebSocket nem dependência nova | resolvida (2026-11-05) | doc 04, seções 2.5 e 9 |
| 26 | Plugin captura e envia por evento (`ItemContainerChanged`, `VarbitChanged`, `StatChanged`) com debounce de ~1 s; a leitura periódica fica como rede de segurança | resolvida (2026-11-05) | doc 06 |
| 27 | Painel da rota dentro do plugin (lista a partir do cursor, bloqueados, objetivos, itens do trecho, Feito/Pular/Desfazer), implementado direto no visual do RuneLite, sem passar pelo canvas; o servidor continua a fonte da verdade e o plugin só guarda o último estado para exibir | resolvida (2026-11-05) | doc 06; doc 04, seção 9 |
| 28 | Auth do plugin: em `AUTH_MODE=dev` sem token (ingest e painel aceitos de `127.0.0.1`, plugin configurado só com a URL); em `oidc`, Device Authorization Grant no Zitadel com uma aplicação nativa pública (sem DCR) e roles do projeto no JWT (`ingest`, `items:*`, `marks:write`), aposentando o token `ipl_`; o registro por instalação fica com o estado do protocolo | resolvida (2026-11-05) | doc 04, seções 2.5 e 8; doc 06; doc 07 (revê "o plugin continua com token `ipl_`") |
| 29 | Comparar rotas é uma aba de Rotas (Oziris · BRUHsailer · Comparar), não a tela principal; a lista de rotas deixa de existir | resolvida (2026-11-05) | docs/ui/decisoes-canvas.md, seção 5 |
| 30 | O produto se chama **Chronicle** (wordmark Alegreya 700 com o livro e a runa); "Ironman Planner" vira descrição | resolvida (2026-11-05) | docs/ui/decisoes-canvas.md, seção 5 |
| 31 | Recorrentes (birdhouses, herb run, kingdom) como entidades com `ready_at` no doc 09 | resolvida (2026-11-10, o dono pediu os passos do roadmap): os relógios do Time Tracking chegam pelo ingest (`data.timers`, faceta `timer`) e o Agora mostra os Recorrentes (V6AgoraEstados1440); kingdom e o que o Time Tracking não conhece ficam para depois | docs/ui/decisoes-canvas.md, seção 5; doc 06, adendo de 2026-11-10 |
| 32 | Modelo de execução: fatos por fonte com junção pura, comandos do dono como log, projeções reconstruíveis, tarefas tipadas, receitas por objetivo, planos e executor no plugin | E1 a E4 **resolvidas** (2026-11-05: fatos por fonte, log de comandos, rebuild no CI e aviso no boot, contrato por fixtures; a geração de tipos foi adiada, com OpenAPI como candidato no lugar do tygo); E5 a E10 **resolvidas** (2026-11-13, do jeito recomendado: as primitivas iniciais com `quest` depois, receitas no git, planos no banco, gravação por demonstração opt-in, nada com risco de morte sem supervisão em HCIM, Quest Helper avaliado na fase 5); E19 e E20 **resolvidas** (2026-11-14: executar quests por delegação ao Quest Helper que o cliente do Microbot traz, com o término provado pelo ingest; não importar nada do Quest Helper); E11 a E18 e E21 **resolvidas** (2026-11-14, o dono confirmou o que estava implementado: a gravação só das ações do dono, guardada até ele apagar, e no conteúdo só os eventos de cada tarefa confirmada; `travel` sem teleporte e `bank` que deposita antes de sacar; `train` recusado com o plugin ligado, as opções gravadas ficam, o plugin só por receita ou plano; a `quest` recusa o bot ocupado e requisitos sem prova); as fases 1 a 5 do doc 10 entram na sequência do doc 04 depois do item 20; o princípio 6 do doc 10 foi revisto pela decisão 41 (2026-11-24) | doc 10 |
| 33 | A rota do Oziris pode ser corrigida pelo dono: notas de update que tornam um passo impossível ou fora de ordem viram patches de import (`data/guide/route-patches.json`: remover, mover, inserir), não só anotações; primeiros: Lost Tribe depois do Goblin Diplomacy e 1.4.2 (favour) removido | resolvida (2026-11-05) | docs/05, seção 12 |
| 34 | Workers do Microbot: cada conta pode ter um worker, no zaros (agent anexado) ou num pod do saradomin criado e apagado pelo Chronicle (web e CLI) no namespace `chronicle`; tudo endereçado pela conta; ao conectar a conta (`chronicle accounts connect`) a sessão vai cifrada para o Postgres e a conta fica habilitada para gerenciar; perfis do cliente (plugins, configurações, gráfico, com modelos), rotinas e histórico no Postgres; qualquer conta, com ou sem rota (modo `regular`); GPU ligada com gráficos no mínimo | resolvida (2026-11-12, revista no mesmo dia com as respostas do dono) no desenho; protótipo no canvas (página 7) a completar com conectar conta, perfil e desvio; W1 a W3 e W8 a W10 abertas; W7 resolvida (2026-11-12: a config do perfil vai pelo Agent Server antes de ligar os plugins); W4 resolvida pelo repositório único (decisão 35) | doc 11 |
| 35 | `chronicle` e o antigo `microbot-plugins` num repositório só: os plugins (Chronicle Link, Cat Swap e os bots), o harness e o projeto Gradle ficam em `microbot/`, com o histórico, e um flake na raiz serve os dois lados (doc 07). A separação tracking × bot continua entre plugins (docs 06 e 08) | revista (2026-11-12): era "repositórios separados", com o acoplamento num contrato, num aperto de mão de versões da ponte e no bump da base do worker entre os dois. Agora um repositório, porque a visibilidade dos repositórios não importa para o dono, toda sessão precisava dos dois, o contrato entre o plugin Java e o servidor Go (ingest, ponte de controle, variáveis do Chronicle Link) muda num commit só e o bump da base entre repositórios (PR e token) some. O dono arquiva o `microbot-plugins` | doc 07, seções 3 e 5; doc 11, seções 7.2 e 10 |
| 36 | Web v7, **Frota**: o produto passa a ser para várias contas. A casa do app é a Frota (uma raia por conta, na ideia de um idle game, com os sprites do jogo); cada conta abre com cabeçalho e cinco abas (Visão geral, Rota, Personagem, Cliente, Histórico); Automação deixa de ser uma área (Workers vira a Frota, o Cockpit vira a aba Cliente, Plugins e Rotinas são da frota); Fila e Histórico juntam relógios, fazer agora, runs e auditoria; Nova conta no lugar do primeiro acesso. Tema e tokens do Pergaminho não mudam; mudam os componentes (cartão no lugar do painel com placa, objetivo com progresso, lâmpada, quadro, barra de andamento) | resolvida (2026-11-12) no desenho: 18 artboards na página 8 do canvas, aprovados pelo dono. Implementação em fatias, item 20 do doc 04; o que depende do servidor (ritmo de XP, fila de rotinas, histórico, perfis, capacidade) vira issue e segue as fases F5 a F7 do doc 11 | docs/ui/decisoes-canvas.md, seção 7 |
| 37 | Worker numa versão antiga: o servidor o reinicia sozinho na imagem nova, mas só na **pausa**: sem run em andamento, sem VNC aberto, e com o personagem deslogado ou num break do Break Handler. Nunca durante um run, nunca com o dono olhando ou intervindo pelo VNC, nunca um worker parado nem o agent anexado do zaros; auditado como o Reiniciar do dono, pelo `reconciler`, com o motivo "versão nova, na pausa". O Reiniciar segue para "agora". Até então, só o clique do dono trocava a imagem de um worker que já roda | resolvida (2026-11-13) e feita no mesmo dia. Falta conferir num worker de verdade. O dono aceitou que o worker reiniciado no break do Break Handler fica na tela de login (W3) em vez de voltar ao jogo (uma vez por imagem nova); o micro break com o personagem logado não conta | doc 11, seções 7.2 e 18.4; docs/ui/decisoes-canvas.md, seção 7.15 |
| 38 | Marca manual por **objetivo**, da conta inteira: o dono marca um objetivo como já cumprido (`chronicle objective mark`), e ele vale em todo guia e passo. Sobrepõe os vereditos ao derivar status, bloqueios e cursor; não é evidência (não escreve `objective_progress`) e não dispara a cascata `implied` | resolvida (2026-11-13): os objetivos de item novos em passos antigos, com os itens já gastos, faziam os passos voltarem a `pending` e o cursor recuar; marcar o passo não resolvia, porque a marca é por guia | doc 09, seção 5 |
| 39 | Marca manual por **item do trecho** (o cartão "Levar do banco"): o dono marca um item que falta como "já tenho" (`chronicle loadout mark` e `mark_loadout_item` no MCP; o cartão só mostra a marca e a desfaz), e a marca vale só naquele trecho de um guia: a mesma pá no trecho seguinte, ou no outro guia, não é marcada. Sobrepõe o veredito do item ao calcular o readiness (a contagem "Falta N de M" cai); não é evidência: não escreve inventário, banco, snapshots nem `player_state`, e o fato do plugin segue como estava | revista (2026-11-15): o botão "Já tenho" saiu do cartão "Levar do banco", que diz o que tirar do banco e não faz sentido marcar; na tela, "Já tenho" é a marca por objetivo (decisão 38) dos objetivos de item do passo atual. Era (2026-11-14): itens que o dono já tem, ou que o plugin não enxerga, apareciam como faltando e não havia como dizer o contrário; marcar o objetivo não alcança os itens do setup do trecho, que não são objetivos | doc 09, seção 5 |
| 40 | Rotina recorrente que falha: **tenta de novo sozinha uma vez**, 15 min depois da falha, com as condições de sempre (ligada, um run por vez, ponto seguro, a pausa do Break Handler), sem esperar o relógio nem o intervalo. Se essa tentativa falhar também, a rotina fica segura até o dono liberá-la (salvá-la de novo, o "Liberar"), como antes. Um run que o dono **cancelou nunca tenta de novo**: a rotina fica segura. O último run da conta que falhou segura todas as rotinas, menos a dona dele (a primeira falha dela não bloqueia a própria tentativa, e as outras esperam essa tentativa); um run que falhou e não é de uma recorrente ("Fazer agora", gatilho, tarefa) segura todas até o dono salvar alguma, como antes. A rotina segura e a tentativa pendente, com a hora, aparecem em Precisa de você (Frota e Atividade), no aviso do topo de Rotinas e no estado da rotina | resolvida (2026-11-14), e feita no mesmo dia: o gesto para soltar uma rotina presa estava em aberto (até então, toda falha ou cancelamento segurava a rotina até o dono salvá-la de novo), e o artboard `V7Rotinas1440` já dizia "Se falhar: tenta de novo em 15 min e avisa em Precisa de você"; o Liberar continua, para a segunda falha e para o cancelamento | doc 08, Estado (travas das recorrentes); doc 11, seção 16.4; `docs/ui/decisoes-canvas.md`, seção 7.13 |
| 41 | Modelo de linguagem em execução, só num passo travado. Um passo de quest está travado quando o varbit da quest não anda depois de N tentativas ou M minutos. Só dentro desse passo um modelo pode agir, só pela lista fechada de comandos do agent (doc 08) e com um orçamento de ações; o passo só termina quando o varbit da quest prova. A escada de escalonamento tem quatro degraus, nesta ordem: o override gravado de (quest, passo), o modelo local (Ollama no zaros), o Claude com orçamento por passo e por dia, e o dono. Fora do passo travado continua valendo "em execução só roda código verificado" | resolvida (2026-11-24): revê a decisão 32 no princípio 6 do doc 10 ("o LLM é preparação, nunca execução") e na seção 11 (uma falha em execução nunca chama um modelo). A conta autônoma precisa de um modelo para destravar passos de quest que o Quest Helper não resolve | doc 10, seções 2 e 11; doc 12, seções 5.7 e 7.5 |
| 42 | **Lugar** de cada conta, escolhido pelo dono: o cliente do jogo roda num pod do saradomin (`cluster`) ou numa vaga do agent do zaros (`workers.placement`). O agent do zaros tem vagas que o servidor preenche com a sessão guardada, como num pod, no lugar da lista fixa de contas (`MICROBOT_AGENT_CLIENTS`). Mudar o lugar de um worker que roda é permitido: o reconciliador tira o cliente do lugar antigo e o sobe no novo, na tela de login. Na tela, um seletor "Rodar em: saradomin \| zaros" no cabeçalho da conta | resolvida (2026-11-26, FEL-138) e feita no mesmo dia no servidor, no agent e na CLI; conferida no zaros com um cliente falso. Falta conferir com um cliente de verdade, no saradomin (oidc) e no serviço do zaros (a opção de vagas do nix-config). O seletor espera o desenho no canvas (regra 11) | doc 11, seção 20 |

Decisões de design tomadas pela equipe e ainda válidas, sem decisão do dono: ids dos steps do
ironman.guide preservados e progresso sem FK para conteúdo; snapshots brutos append-only mais
`player_state` como projeção da junção por fonte (doc 10, E1; até 2026-11-05 era "última escrita vence" com prioridade plugin > wikisync > hiscores > wom); motor puro com vereditos
met / unmet / unknown e evidência; skills no grafo como limiar de aresta; protocolo do plugin clone do
WikiSync.

## Modo de execução

Local-first: o MVP roda no `zaros` com Docker Compose (Postgres + binário) e `AUTH_MODE=dev`. O deploy
no `saradomin` com Zitadel vem depois do marco "em uso", com a mesma imagem. Toolchain: Go 1.27, Node
22, pnpm, Nix flake, Docker. Ver doc 04, seção 8, e doc 07.

## Controle do Microbot

Além do tracking somente leitura (doc 06), o dono decidiu controlar o cliente Microbot pelo planner:
headless no zaros, VNC no navegador, plugins do Hub e rotinas agendadas. Módulo separado e desligado
por padrão, com risco aceito conscientemente. Ver doc 08. Em 2026-11-12 o dono decidiu ter também
workers headless no saradomin, um por conta, escolhidos no painel (decisão 34, doc 11).
Em 2026-11-26 passou a escolher, por conta, se o cliente roda num pod ou numa vaga do zaros
(decisão 42, doc 11, seção 20).

## Próximo passo

Fica no Linear, no projeto Chronicle (<https://linear.app/felipemalacarne/project/chronicle-1f382104f1b7>).
As pendências que esta seção listava em 2026-11-13 viraram as issues FEL-51 (com FEL-106 a FEL-110),
FEL-52, FEL-53, FEL-55, FEL-56 e FEL-112.

## Discrepâncias e ressalvas

- O doc 03 estimou cobertura por inspeção dos campos; o doc 05 mediu após inferir regras do texto. Em
  caso de conflito, vale o doc 05.
- 7 nomes de quests do guia diferem do wiki (ex.: Vampire Slayer vs Vampyre Slayer, Garden of
  Tranquility vs Tranquillity); o normalizador já renomeia e `quests.json` carrega aliases.
- Dois steps iniciam quest sem step explícito de conclusão (Barcrawl, Fairytale II); comportamento
  esperado do guia original.
- O guia nunca cita kill count, então `boss_kc` só será útil em metas e no 2.0 com regras escritas à mão.
- O doc 00 é histórico e não é mantido; os docs 02, 03, 04 e 09 o substituem.
