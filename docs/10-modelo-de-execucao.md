# 10 — Modelo de execução: fatos, eventos, tarefas, receitas e planos

**Estado: proposta (2026-11-05). E1 a E4 decididas pelo dono em 2026-11-05 e E5 a E10 em 2026-11-13, do
jeito recomendado (seção 14); E11 a E18 em aberto, com o conservador implementado. As fases 0 (doc 04,
seção 9, item 17) e 1 (item 21, 2026-11-13) estão feitas. O código das fases 2, 3 e 4 está pronto
(2026-11-13). Falta a parte do dono: na 2, gravar um trecho do Oziris 1.1 e confirmar as tarefas
propostas; na 3, executar um trecho de logística no jogo, olhando, com o término provado pelo ingest;
na 4, um treino até `skill:x>=N` no jogo sem intervenção (seção 13).** A fase 1
entregou as entidades `npc`, `shop` e `object`, as áreas dos locais, o schema de `tasks[]` e de
receitas e o verificador (seções 7 e 8, "Como implementado"). A fase 2 entregou a gravação por
demonstração no plugin e no ingest, as tarefas propostas a partir dela e o modo assistido no painel
(seções 10 e 11, "Como implementado"). A fase 3 entregou os planos (seção 9), o comando de tarefa no
agent e o executor de `travel`, `bank` e `buy` no plugin (seção 10), todos com "Como implementado". A
fase 4 entregou o `wait_until` por chave de objetivo nos runs e nas rotinas do doc 08 e o `train` por
delegação a um plugin (seção 10, "Como implementado (fase 4)").
Ainda não há tarefa ou receita real no conteúdo: elas entram com a gravação e a confirmação do dono. O
resto deste doc não está implementado. Ele estende o doc 09 (o modelo de objetivos continua valendo) e o doc 08 (o controle do
Microbot ganha um modelo de tarefa). Diagramas: página "Chronicle · mapa da arquitetura"
(<https://claude.ai/artifact/EZaKJZKjVAeotQG41rehEU>), seções 5 a 8.

## 1. Por que agora

Três motivos, na ordem em que apareceram na revisão de 2026-11-05:

1. **Consistência.** A revisão do backend e da web achou invariantes que só o Go garante, corridas
   entre o ingest e o sync do hiscores e um contrato API ↔ web mantido à mão (seção 4). O núcleo do
   doc 09 está certo; o que falta é a camada em volta dele.
2. **Automação.** O dono quer, além de acompanhar a conta contra a rota, executar planos pelo plugin
   no Microbot (Rs2Walker, Rs2Bank e afins), estendendo o nosso plugin ou criando outros. Os passos são
   texto natural; o doc 08 só prevê rotinas de plugins prontos.
3. **Mais rotas, rotas próprias e mais personagens.** O conhecimento de como cumprir um objetivo não
   pode morar no texto de um guia, senão é reescrito por rota e por conta.

## 2. Princípios

1. **O mundo continua compartilhado e as rotas independentes** (doc 09, princípio 1; CLAUDE.md regra 7).
2. **Fonte de verdade só de adição onde a verdade é um fato que aconteceu**: observações das fontes,
   comandos do dono e execuções da automação. Todo o resto é projeção.
3. **Projeções síncronas e reconstruíveis.** Calculadas na mesma transação do evento, por função pura
   em `internal/core`, e conferidas por `chronicle progress rebuild --check`.
4. **O banco é a última linha de defesa.** Cada invariante do doc 09 vira CHECK, FK ou trigger, além do
   código Go.
5. **Uma linguagem de predicado só.** A chave de objetivo (doc 09, seção 3.2) é o término do passo, da
   meta, da tarefa e da rotina.
6. **O LLM é preparação, nunca execução.** O agente transforma texto em tarefas e receitas uma vez por
   texto, com verificação determinística e confirmação do dono (CLAUDE.md regra 9). Em execução só roda
   código verificado.
7. **Nada concluído sem evidência, nem pelo bot** (CLAUDE.md regra 2). A tarefa termina quando o
   ingest prova o objetivo, nunca porque o executor diz que terminou.
8. **`manual` é o padrão.** Toda tarefa sem tradução tipada é manual e continua válida; a automação
   cresce passo a passo.

## 3. O que fica do doc 09

Ficam sem mudança: entidades e chave de objetivo canônica, papéis no passo (`completes`, `starts`,
`requires`, `uses`), `ItemRequirement`, sidecars com proveniência, status do passo derivado e não
gravado, veredito `met` travado com evidência, cascata `implied` com os limites da seção 9.1, cursor
por guia.

## 4. Achados da revisão (2026-11-05)

Base verificada: `go build`, `go vet` e `go test ./...` verdes; testes de store e httpapi verdes num
Postgres descartável com as migrations 0001 a 0006 (Postgres 16 no ambiente; o CI usa 17).

| # | Achado | Onde | Gravidade |
|---|---|---|---|
| R1 | Trava do `met` só no Go: `UpsertObjectiveProgress` sobrescreve o veredito sem condição | `db/queries/progress.sql` | alta |
| R2 | Evidência de `met` pode ser `[]`: o CHECK não exige array não vazio | `0004`, `objective_progress_met_has_evidence` | média |
| R3 | Cascata do motor e marca do dono usam a mesma `UpsertStepMark`; só o Go impede o motor de sobrescrever o dono | `progress.sql`, `reevaluate.go` | alta |
| R4 | Ingest e sync do hiscores gravam `player_state` antes de travar a conta, em ordens diferentes: risco de deadlock | `store/ingest`, `store/syncrun` | alta (não reproduzido) |
| R5 | `observed_at` vem do relógio do plugin; `player_state` não tem a fonte na chave e vence a última escrita | `0001`, `normalize.go` | média-alta |
| R6 | Dois syncs simultâneos colidem no índice único e um vira `error` com 5 min de espera | `syncrun.go` | média |
| R7 | `guide_id` é texto livre em contas, marcas e cursor; snapshot de evidência não amarrado à conta | migrations 0001 a 0004 | média |
| R8 | Chaves e status como string solta depois do parse (`map[string]`, `Cause`, `RunStatus`) | `core/progress`, `store/syncrun` | média |
| R9 | Undo do Agora sem a guia: desfaz no `guideId` atual, e ids se repetem entre guias | `web/src/routes/+page.svelte` | alta |
| R10 | Escritas sobrepostas (`pin`, undo em lote sem `busy`) e board da rota anterior alimentando a nova | `+page.svelte`, `resource.svelte.ts` | média-alta |
| R11 | `types.ts` escrito à mão já divergiu do Go; a web reimplementa `isDone`, `pendingBefore` e distâncias | `web/src/lib/progress` | média |

## 5. Fatos: observações por fonte e junção pura

Hoje `player_state` tem chave `(account_id, facet, key)` e guarda o último valor de qualquer fonte. Proposta:

- **`observations`** com chave `(account_id, facet, key, source)`: o último valor de cada fonte, com
  `observed_at` limitado ao relógio do servidor (`min(sent_at, now)`) e o `snapshot_id`.
- **`player_state` vira projeção** calculada por uma função pura `facts.Merge` em `internal/core`, com
  testes de tabela:
  - **fatos que só crescem** (XP e nível, estado de quest, diary, unlock, KC, CA, collection log): o
    maior valor entre as fontes. A ordem de chegada e o relógio deixam de importar e o replay é
    determinístico;
  - **itens**: continuam com o frescor por container e época (doc 04, seção 6.4.1; CLAUDE.md regra 8).
- A prioridade por fonte prometida no comentário de `player_state.sql` some: o valor máximo resolve os
  fatos monotônicos e o frescor resolve os itens.

## 6. Eventos e projeções

**Logs, só adição:**

| Log | Conteúdo | Escritor |
|---|---|---|
| `snapshots` + `snapshot_facts` | observações brutas e normalizadas | ingest, sync (já existe) |
| `mark_commands` (como implementado; o rascunho chamava `owner_commands`) | toda mudança de `step_marks`, com os efeitos (marca antes e depois de cada passo): comandos do dono (marcar, pular, fixar, marcar antes, desfazer em lote, confirmar `implied`; `via` web ou painel do plugin; `revokes_id` aponta para o comando desfeito), as marcas `implied` do motor (autor `engine`) e as mudanças de id por alias (autor `system`); só adição, garantido por gatilho | API do dono, painel do plugin, motor, boot |
| `run_events` | tarefa iniciada, concluída, entregue ao dono, falhou; com screenshot e estado | executor (seção 10) |

**Projeções**, na mesma transação, por função de `(logs, conteúdo atual)`: `player_state`,
`objective_progress`, `step_marks` (com `command_id`), `route_cursor`, status do passo. O desfazer passa
a ser um comando novo que revoga o anterior pelo id, e não uma regravação de estado (resolve R9 por
construção).

**Autor.** Toda evidência e todo evento identificam o autor. Como implementado (fatia d, 2026-11-05):
as marcas têm `actor` e `via` em `mark_commands`, e os eventos de status apontam para o comando
(`step_status_events.command_id`); o autor de um veredito é a fonte do snapshot da evidência
(`snapshots.source`, amarrado à conta pela FK composta da 0007), sem coluna repetida. `automation` e
`run_id` entram com o executor (fases 3 e 4).

**Reconstrução.** `chronicle progress rebuild --check` recalcula as projeções do zero e mostra a
diferença para o banco. Roda no CI contra fixtures e pode rodar no boot. A trava do `met` de item usa
`observed_at` limitado, nunca a ordem de inserção.

Como implementado (fatia e, 2026-11-05), o comando é `chronicle progress check --account <id>`: fala
com `GET /api/progress/check` de um servidor no ar (CLAUDE.md regra 1) e sai com 1 se houver
diferença. Numa transação só de leitura, o servidor confere três coisas: `step_marks` contra a
repetição de `mark_commands`; `player_state` contra a junção das `observations`; e vereditos, marcas
`implied`, últimos status e cursores contra uma reavaliação dos fatos atuais, que não pode ter nada a
gravar. Não repete o histórico de snapshots desde o início: a cascata `implied` depende da ordem em
que os vereditos mudaram, e esse histórico já está nos logs de eventos. No boot, a checagem roda antes
da reavaliação e só avisa no log; no CI, os testes de integração do store terminam com ela, num
Postgres 17 do job `go`.

**Distribuição.** Um `outbox` gravado na transação alimenta o hub. Com um processo, o hub em memória
de hoje basta (decisão 25 do doc 01). Com o `serve` e o agent em processos separados, o hub lê o
outbox por `LISTEN/NOTIFY`. Sem broker.

**Restrições no banco** (resolvem R1 a R3, R7): `DO UPDATE ... WHERE verdict <> 'met' OR EXCLUDED.verdict = 'met'`
ou trigger equivalente; `jsonb_array_length(evidence) > 0` (`NOT VALID` e depois `VALIDATE`);
`InsertImpliedMark ... ON CONFLICT DO NOTHING` só para o motor; tabela `guides` preenchida no boot pelo
`IndexContent`, com FKs; `UNIQUE (id, account_id)` em `snapshots` e FKs compostas nas evidências;
`CHECK` de formato em `objective_key`.

**Concorrência** (R4, R6): ordem global de locks credencial → conta → fatos, tomada no início de toda
transação que grava progresso; `pg_advisory_xact_lock` por conta no sync.

## 7. Tarefas: a linguagem de execução

Campo `tasks[]` no passo, vindo só de anotações (como `objectives[]`), e nas receitas (seção 8). Cada
tarefa tem tipo, parâmetros resolvidos pelo resolver e uma condição de término que é chave de objetivo
ou predicado de estado avaliável pelo motor.

| Tipo | Parâmetros | Termina quando | Primitiva no plugin |
|---|---|---|---|
| `travel` | `location`, `via?` (unlock, item de teleporte) | posição dentro da área do local | `Rs2Walker`, teleportes |
| `bank` | loadout do trecho (já derivado) | inventário e equipamento atendem ao loadout (readiness) | `Rs2Bank`, `Rs2Inventory` |
| `buy` | `shop`, `item`, `qty` | `item:<id>xN` | `Rs2Shop` |
| `talk` | `npc`, opções de diálogo? | objetivo do passo | `Rs2Npc`, diálogo |
| `interact` | `object`, ação | objetivo do passo | `Rs2GameObject` |
| `train` | `skill`, método (npc, spell, objeto), local | `skill:x>=N` | delega a um plugin ou primitiva própria |
| `quest` | `quest` | `quest:x@finished` | receita própria ou dados do Quest Helper; avaliado no apêndice A: delegar ao Quest Helper do Microbot |
| `manual` | texto citado | marca do dono ou objetivo do passo | nenhuma: o executor para e chama o dono |

Regras:

1. **Sem texto para interpretar.** Uma tarefa só tem refs do catálogo, números e ids; a citação do
   texto fica como evidência, igual aos objetivos (doc 09, seção 4.1).
2. **Idempotência.** Antes de agir, a tarefa avalia a própria condição de término e pula se já está
   cumprida. Por isso a mesma tarefa serve para conta nova, conta adiantada e retomada.
3. **Pré-condições checáveis.** `requires` e itens avaliados pelo motor e pela readiness. Se falta
   algo, o plano insere a tarefa que produz o que falta, quando houver receita; senão, `manual`.
4. **Verificador.** Aceita a tarefa só com refs existentes, primitiva conhecida pelo executor e término
   avaliável. As inferências seguem a CLAUDE.md regra 9 (`confirmed_by: owner` ou decisão registrada).

Tipos de entidade novos para isso: `npc`, `shop` (com estoque), `object`; e `location` ganha área em
coordenadas. Todos vêm da wiki, como os itens (doc 09, seção 7).

**Como implementado (fase 1, entidades e áreas, 2026-11-13).** Detalhes no doc 09, seções 3.1 e 7.

- `npc`, `shop` e `object` são refs de slug do título da página da wiki (`npc:duke_horacio`,
  `shop:bobs_brilliant_axes`, `object:bank_booth`), com catálogos gerados da wiki inteira pelo
  `catalogbuild`: 5.435 NPCs (com os ids de NPC de todas as versões, que o executor procura no jogo, e as
  páginas da localização do infobox), 462 lojas (donos e estoque: item, estoque base ou infinito, preço e
  moeda, 4.914 linhas resolvidas e 461 guardadas como não resolvidas, com o motivo) e 5.898 objetos (com os
  ids de objeto). Fonte: as tabelas Bucket da wiki (`tools/extract_wiki_world.py`, extrato em `data/wiki`).
- O resolver resolve os três por nome, alias e título da página, como os itens: quando várias páginas têm
  o nome, vale a página com aquele título ("Banker"); senão é ambíguo ("Cook", "Crossbow Shop") e os
  candidatos vão no resultado, nunca um palpite. O `resolve_entity` do `content mcp` já aceita os três.
- **Regra da área do local** (`entities.Area`, em `location_maps.json`): o polígono do mapa do infobox da
  wiki, senão o retângulo ou o quadrado do mapa, senão a caixa de 21 por 21 tiles em volta do ponto do
  mapa (`AreaRadius = 10`). A caixa padrão é pequena de propósito: `travel` termina perto do ponto que a
  wiki marca, e é melhor andar a mais do que parar antes. Local sem mapa (região, instância) não tem
  área, então `travel` para ele não tem término avaliável. Hoje: 11 polígonos, 1 retângulo, 5 quadrados e
  74 caixas padrão, de 91 locais com mapa; 14 sem mapa.
- O que fica para depois: localização dos objetos (o infobox de scenery não tem) e os pontos exatos de
  NPCs e objetos (a wiki tem mapas por página, não lidos); entram quando o executor precisar (fase 3).

**Como implementado (fase 1, tarefas e verificador, 2026-11-13).**

- **Schema**: `content/schema/task.schema.json`, uma tarefa por tipo da lista de primitivas (E5:
  `travel`, `bank`, `buy`, `talk`, `interact`, `train`, `manual`), cada tipo com os seus parâmetros
  (tabela acima), e o término em `until`: `{"objective": <chave>}` ou `{"state": ...}`, com quatro
  predicados de estado: `at_location` (posição na área do local), `loadout_ready` (o setup atende ao
  loadout), `goal_met` (os objetivos `completes`/`starts` do passo, ou a chave da receita, estão `met`) e
  `owner_mark` (a marca do dono, só `manual` num passo). `travel`, `bank` e `buy` têm término padrão
  (`at_location`, `loadout_ready`, `item:<id>x<qty>`); os outros declaram. `options` do `talk` e `action`
  do `interact` são o texto do próprio jogo (opção de diálogo, opção de menu), comparado por igualdade e
  nunca interpretado; `note` do `manual` é texto para o dono. Feitiço como método de `train` fica para
  quando houver catálogo de feitiços.
- **No passo**: `tasks[]` só nos sidecars de anotação (`annotations.schema.json`, `annotatedTask`: a
  tarefa mais `evidence` com citação e `source`, como os objetivos), nunca no `guide.json`. O loader
  verifica e compõe em `Step.tasks`; `Content.TasksOf(passo, modo)` junta as tarefas herdadas das receitas
  dos objetivos que o passo conclui com as do próprio passo.
- **Verificador** (`content.Task.Check`, regra 4), rodado pelo loader, então pelo `content validate` e pelo
  boot do `serve`:
  - tipo da lista de primitivas; `quest` é recusado como "ainda não é primitiva" (E5);
  - só os parâmetros do tipo; cada ref existe no catálogo e é do tipo certo;
  - a loja de `buy` vende o item pelo estoque da wiki;
  - o destino de `travel` tem área;
  - `bank` sem itens só num passo cujo trecho tem loadout com itens;
  - término avaliável: a chave existe no catálogo, `train` termina na própria skill, `goal_met` exige um
    objetivo que o passo conclui, `owner_mark` só em `manual` de passo;
  - no passo, a citação confere com o texto e uma inferência tem `confirmed_by: owner` ou decisão, como
    nos objetivos.
- **Ferramental**: o `revise_entity` do `content mcp` acha e renomeia os refs das tarefas dos sidecars. Desde
  a fase 2, o `submit` aceita tarefas e o `submit_tasks` grava só elas (seção 11, "como implementado").

**Como implementado (fase 2, evidência gravada, 2026-11-13).** A evidência de uma tarefa de passo
(`content.TaskEvidence`) é a citação do texto, os eventos gravados que a sustentam (`recorded`: o id da
gravação e os eventos, como o plugin os gravou, sem a conta) ou os dois. A proveniência ganha o método
`recording` (tarefa proposta da gravação). O verificador (`content.CheckRecorded`, rodado pelo loader)
confere que os eventos são válidos, de uma gravação só, e mostram a tarefa pelos ids do catálogo:

- `travel`: uma posição gravada dentro da área do local;
- `talk`: um clique no NPC (um dos ids de jogo dele) e as opções de diálogo, na ordem;
- `interact`: o objeto com a mesma opção de menu;
- `buy`: a compra do item, e o lojista gravado é dono da loja;
- `bank`: um evento de banco;
- `train`: o clique no NPC ou no objeto.

Tarefa com evidência gravada sempre exige `confirmed_by: owner`: a gravação mostra o que o dono fez, não o
que o passo pede.

## 8. Receitas por objetivo

Uma receita diz como cumprir uma chave de objetivo, independentemente da rota:

```
recipe quest:the_grand_tree@finished
  requires: (do catálogo de quests)
  modes:    ironman, hcim (variantes quando algo muda)
  risk:     combat (Black demon)        → bloqueia execução sem supervisão em HCIM
  tasks:    travel(location:gnome_stronghold) → talk(npc:king_narnode) → …
```

- Vivem em `content/recipes/`, versionadas no git, com o mesmo verificador e proveniência dos sidecars.
- Um passo que conclui uma chave com receita herda as tarefas dela; as tarefas do próprio passo ficam
  só para o que é da rota ("fire strike gnome guards até 43").
- As receitas são o que torna rotas novas e customizadas baratas: o agente só extrai o que nenhuma
  receita cobre.
- Recorrentes (decisão 31 do doc 01: birdhouses, herb run) cabem aqui como receitas com `ready_at`.

**Como implementado (fase 1, 2026-11-13).** Schema `content/schema/recipe.schema.json`, formato e regras
em `content/recipes/README.md`. Um arquivo por chave, com o nome derivado dela
(`quest:cooks_assistant@finished` vira `quest-cooks_assistant-finished.json`): `objective`, `requires`
(chaves além dos requisitos do catálogo), `variants` (por modo de conta: `ironman`, `hcim`, `uim` e
`gim`; um modo em uma variante só), `risks` por variante e `source`. Riscos numa lista fechada (E9):
`combat`, `wilderness` e `hazard` podem matar, `item_loss` não; `RecipeVariant.Unsupervised(modo)` diz
que em HCIM uma variante com risco de morte não roda sem supervisão, e o executor (fase 3) aplica. O
verificador (`content.CheckRecipe`) confere:

- o nome do arquivo e uma receita por chave;
- chave e requisitos conhecidos;
- variantes e modos;
- riscos da lista, com refs conhecidos;
- cada tarefa por `Task.Check`: numa receita, `goal_met` é a chave dela, `bank` precisa de `items` e
  `manual` de `note`;
- a última tarefa de cada variante termina no objetivo da receita;
- inferência confirmada pelo dono. Receita não cita decisão, porque não é de um guia.

`content/recipes/` fica só com o README: as receitas entram pela extração e pela gravação (fase 2). O
formato, o loader e o verificador estão provados com fixtures em `internal/core/content/testdata/execution`:
um guia com tarefas nos passos e três receitas válidas, que juntos usam os sete tipos, e 33 casos
inválidos em tabela (16 de tarefa de passo e 17 de receita), além da tabela do verificador em Go com 37
casos.
`content validate` informa quantas tarefas e receitas leu.

## 9. Planos

O **plano** é a lista ordenada que a conta segue. Cada item referencia um passo de guia
`(guide_id, step_id)`, uma receita ou uma tarefa avulsa; o texto nunca é copiado.

| Tipo | Origem | Onde vive |
|---|---|---|
| `route` | um guia, um para um | gerado do conteúdo; não precisa de tabela até haver edição |
| `goal` | plano de meta (`GET /api/goals/{id}/plan`, hoje calculado na hora) | banco |
| `custom` | montado pelo dono na UI, citando passos de vários guias e receitas | banco; exportável para `content/routes/` quando virar rota oficial |

O cursor passa a andar sobre o plano ativo da conta. Para `route`, é o cursor de hoje. A regra 7 continua
valendo: um plano customizado referencia passos de guias diferentes sem fundir o conteúdo deles, e a
atribuição de cada passo aparece na tela.

**Como implementado (fase 3, planos, 2026-11-13).**

- **Banco** (migration 0018, escrito só por `internal/store/plans`): `plans` (`kind` em `route`, `goal`,
  `custom`; uma `route` só nomeia o guia, uma `goal` aponta para a meta da própria conta) e `plan_items`
  (em ordem: um passo `(guide_id, step_id)`, uma receita pela chave ou uma tarefa avulsa em JSON;
  CHECK de um ref só por item, e um gatilho recusa itens num plano `route`). `accounts.active_plan_id` é o
  plano que a conta segue; `NULL` é a rota de `accounts.guide_id`, então o plano de rota não precisa de
  linha: ele é o conteúdo e o cursor de hoje, sem cópia. Apagar o plano seguido (ou a meta dele) devolve a
  conta à rota. Um passo que o guia renomeia (`step_id_aliases`) muda também nos planos.
- **Próxima tarefa** (`internal/core/plan`, puro, com testes de tabela): no plano de rota, o passo do
  cursor; nos outros, o primeiro item não cumprido (passo feito ou pulado, receita com o objetivo `met`,
  tarefa avulsa com o término `met`). As tarefas de um passo são as do `Content.StepTasks` (as receitas do
  que ele conclui, pelo modo da conta, e as dele). Cada término é avaliado como no modo assistido; a
  primeira tarefa que não está `met` é a próxima, e as `met` antes dela vêm em `skipped` (regra 2:
  idempotência). `unknown` nunca é pulado. O plano só insere o que sabe verificar (regra 3): para um
  requisito do passo provado `unmet` (`blocked_by`), as tarefas da receita dele, marcadas com `for`; sem
  receita, o item fica com o dono (`manual`). Estados: `task`, `waiting` (tarefas cumpridas, o passo
  ainda não concluiu), `manual` (sem tarefa tipada) e `done`.
- **Cada tarefa** tem uma chave estável (`step:<guia>::<passo>#<i>`, `recipe:<chave>#<i>`,
  `task:<posição>#0`), os riscos (os da variante da receita, mais `wilderness` num `travel` cujo destino
  toca a wilderness pelos limites do webwalker do Microbot), `supervised` (E9: HCIM com risco de morte) e
  se o executor da fase 3 a roda: `travel` sem `via` e com área, `bank` com itens (os da tarefa ou o
  setup do trecho) e nunca para UIM, `buy` de loja com lojista de id conhecido.
- **API** (com `authed` e `accountParam`, regra 10): `GET /api/plan/next?account=` (a próxima tarefa, com
  o crédito do guia no item de passo, regra 6), `GET` e `POST /api/plans`, `PUT /api/plans/active` e
  `DELETE /api/plans/{id}`; na CLI, `chronicle plans` (`create -f`, `use`, `delete`) e
  `chronicle task`. Um plano novo passa por `plan.CheckItems`: o passo existe no guia, a receita
  existe e a tarefa avulsa passa pelo verificador como a de uma receita (sem `goal_met` nem `owner_mark`;
  `bank` com itens). As fixtures `plan-next` e `plans` fixam o contrato com a web, que ainda não tem tela
  de planos (os planos customizados na UI são da fase 5 e pedem o canvas antes).

## 10. Executor, agent e personagens

- **O servidor decide o quê:** a próxima tarefa do plano, já tipada, com riscos e modo da conta.
- **O plugin decide o como:** uma máquina de estados local sobre as primitivas do Microbot. Nada de
  cliques pela rede.
- **O mesmo plugin lê e executa.** O término é confirmado pelo ingest como evidência de objetivo.
- **O agent do doc 08** fica com o ciclo de vida do cliente (abrir, logar, VNC, instalar plugins) e
  ganha um comando da lista fechada: "executar a tarefa tipada X". `/scripts/deploy` e entrada crua
  continuam fora.
- **Plugins de terceiros** entram por delegação: `train` liga o plugin, monitora e desliga quando o
  objetivo fica `met`.
- **Modo assistido antes do automático:** a mesma tarefa primeiro só destaca o NPC ou objeto, desenha
  o caminho e avisa os itens que faltam. Valida a qualidade das tarefas sem risco.

  **Como implementado (fase 2, 2026-11-13).** O primeiro degrau, ainda sem destacar nada no jogo: o
  painel do Chronicle Link 1.14.0 mostra as tarefas do passo atual como uma lista de verificação. O
  servidor monta a lista (`internal/core/panel`, `Window.steps[0].tasks`, só no passo do cursor) com
  `Content.TasksOf` (as tarefas herdadas das receitas e as do passo, pelo modo da conta) e avalia o
  término de cada uma com o que sabe:

  | Término | Avaliado por |
  |---|---|
  | chave de objetivo | o veredito travado do passo, quando é objetivo dele; senão `progress.EvaluateObjective` sobre os fatos da conta |
  | `at_location` | a última posição do plugin ("Send position") dentro da área do local; posição com mais de 15 minutos é `unknown` |
  | `loadout_ready` | a prontidão do trecho (ou dos `items` da tarefa) sobre o inventário e o equipamento ao vivo |
  | `goal_met` | os vereditos dos objetivos que o passo conclui |
  | `owner_mark` | a marca `manual_done` do dono no passo |

  O que o servidor não sabe é `unknown`, com o motivo, nunca "a fazer". Cada linha traz o rótulo
  ("Ir até Lumbridge Castle", "Falar com Duke Horacio: …"), o término por extenso e se veio de uma
  receita. O plugin só mostra (✓ feito, ○ a fazer, ? sem dado, com o término e o motivo no tooltip); a
  `chronicle now` mostra a mesma lista. O JSON está fixado nos dois lados: o `TestAssistedModeWire`
  grava `route-tasks.json` nos recursos de teste do plugin, que o `RouteWindowTest` lê. Destacar o NPC
  ou o objeto, desenhar o caminho e agir ficam para as fases seguintes.
  **Como implementado (fase 3, o comando de tarefa, 2026-11-13).** O servidor decide o quê e o agent
  carrega, sem interpretar (detalhes no doc 08, "Fase 3 do doc 10"):

  1. `POST /api/microbot/task?account=` pega a próxima tarefa do plano ativo (seção 9) e recusa com
     409, sem falar com o agent: sem tarefa a fazer, tarefa diferente da que o chamador viu, tipo que
     o executor não roda, ou HCIM com risco de morte sem `supervised` (E9).
  2. `plan.Resolve` traduz a tarefa para o `execspec.Task` (`internal/core/execspec`): a área e um tile
     dentro dela, os itens com os ids aceitos, os lojistas e quanto falta comprar. Uma tarefa já `met`
     não roda.
  3. O comando `task_run` da lista fechada valida tudo de novo, E9 incluída, e roda como um run do
     doc 08: ponto seguro, trilha, timeout, cancelamento, histórico e auditoria. Ele inicia a máquina
     do Chronicle Link pela ponte e acompanha as linhas dela.

  O `done` do run é o plugin terminando a máquina, nunca a tarefa cumprida: `GET /api/microbot/task`
  devolve a `proof`, a tarefa avaliada agora sobre o que o ingest provou, e `proven` só com o término
  `met`. Sem tela na web (o botão "Executar" pede o canvas): a CLI `chronicle task` lista, executa,
  acompanha e cancela.

  **Como implementado (fase 3, o executor no plugin, 2026-11-13).** Chronicle Link 1.15.0 (doc 06,
  adendo de 2026-11-13 sobre o executor). A máquina de estados roda numa thread do plugin, só no cliente
  do harness e só quando o agent pede pela ponte (`/bridge/task/start`, `/bridge/task`,
  `/bridge/task/cancel`):
  - `travel` anda com o `Rs2Walker` até a área e para ao entrar nela; fora de um destino na wilderness,
    exige "Avoid wilderness" e para a caminhada que entrar nela;
  - `bank` abre o banco mais perto sem o webwalker (só um banco ao alcance do clique), deposita o que não é do setup, saca o que falta (notado
    quando pedido) e veste o equipamento;
  - `buy` abre a loja pelo lojista mais perto e compra em lotes sem passar do que falta.

  Ela confere cancelamento, pausa dos scripts e login entre os passos, e para na primeira coisa que não
  consegue, com o motivo. Ao terminar, pede uma captura ao rastreamento, para o ingest provar logo o
  término.

  **Como implementado (fase 4, 2026-11-13).** Dois PRs empilhados; detalhes no doc 08, "Fase 4 do doc
  10".

  1. **`wait_until` por chave de objetivo** (princípio 5: a chave de objetivo também é o término da
     rotina). Um run do doc 08 (o "Fazer agora", um gatilho, uma recorrente) ganha
     `until: {objective, account}` e termina quando o servidor prova a chave sobre os fatos da conta,
     nunca porque o plugin ou o agent dizem. O agent reporta o run com o `until` (mensagens `event`);
     o vigia do servidor (`internal/rununtil`) avalia a chave a cada evento de progresso da conta e a
     cada 30 s, só com fato vivo, e manda o comando novo `run_until` (servidor → agent, auditado como
     origem `server`); o agent para o plugin, volta e religa o principal. Prova antes de o run agir
     termina sem agir (regra 2: idempotência); plugin que para sozinho espera a prova por 2 min e
     falha depois.
  2. **`train` por delegação.** A tarefa `train` ganha `plugin` (a classe do plugin do Microbot, um id
     do cliente) e `config` (as opções com que ele começa: grupo, chave e valor, gravados como estão,
     nunca interpretados; mesmas regras do `config_set`, em `internal/core/clientcfg`). Sem `plugin`, o
     treino continua com o dono ("faça à mão"). O `execspec` ganha `train` (skill, nível, plugin,
     opções e se o método luta) e o `plan.Resolve` o resolve. Riscos (E9): `combat` quando o NPC é um
     monstro (o infobox de monstro da wiki) ou a skill só se treina lutando (attack, strength,
     defence, hitpoints, ranged, slayer), `wilderness` quando o local toca a wilderness; HCIM com risco
     de morte só com `supervised`. O `task_run` de um `train` não vai para a máquina do Chronicle
     Link: o agent confere o ponto seguro, o login, a wilderness em HCIM, a ponte respondendo (é o
     rastreamento dela que o ingest usa para provar o nível), que o cliente tem o plugin
     (`/scripts`) e que ele está desligado; grava as opções, liga o plugin e o acompanha como um run
     com `wait_until` em `skill:<skill>>=N`. Termina `done` só com a prova do servidor, que desliga o
     plugin; timeout (padrão 2 h, até 12 h), falha do plugin e cancelamento do dono também o
     desligam. O `POST /api/microbot/task` recusa com 409, antes de tocar no jogo, um plugin que o
     cliente não lista (instalado pelo perfil da conta, pelo modelo da frota ou pelo Hub) e um agent
     sem `run_until`.

  O plugin de um `train` vem de uma receita (`content/recipes/`, por exemplo a de
  `skill:fishing>=76`, que todo passo que conclui essa chave herda) ou de uma tarefa avulsa num plano
  customizado; a extração do texto do guia nunca o põe (E18).
- **Vários personagens:** catálogos e receitas compartilhados; por conta ficam modo, fatos, plano ativo
  e cliente (`clients`). O agent gerencia N clientes, com um executor e um lock por cliente.
- **Risco:** toda receita declara riscos; em HCIM, risco de morte bloqueia execução sem supervisão. O
  módulo continua desligado por padrão (`MICROBOT_CONTROL=off`) e o risco de banimento segue aceito
  conscientemente (doc 01, decisão 9).

## 11. Preparação e execução

O agente roda na preparação, como um compilador: texto vira tarefas e receitas verificadas. Ele volta
só quando:

- o texto de um passo muda (`text_hash` desatualizado, só aquele passo);
- entra uma rota nova, para os passos sem receita;
- uma receita falha e o dono quer corrigir.

Uma falha em execução nunca chama LLM: o executor para, grava `run_events` com estado e screenshot e
entrega ao dono (`manual`).

**Gravação por demonstração.** Enquanto o dono joga um passo, o plugin grava caminho, interações
(`MenuOptionClicked`) e mudanças de banco e inventário. A gravação vira proposta de tarefas ou de
receita e passa pelo mesmo verificador e pela confirmação do dono. É o caminho principal para corrigir
receitas que falharam e para cobrir passos que o texto descreve mal.

Continuam difíceis de tornar determinísticos: diálogos e etapas de quest (avaliar importar do Quest
Helper em vez de extrair do texto) e combate real, que começa como `manual` ou delegado. A avaliação
do Quest Helper (E10, 2026-11-13) está no apêndice A. A recomendação é não importar os dados dele, e
sim executar a quest por delegação ao Quest Helper que o cliente do Microbot já traz, como o `train`.

**Como implementado (fase 2, a gravação, 2026-11-13).** Chronicle Link 1.13.0, opção "Record
demonstration", desligada por padrão (E8). O plugin registra só o que o dono faz, com os ids do jogo,
como eventos tipados (`internal/core/recording`): `move` (posição grosseira: a primeira, outro mundo,
plano ou região, e onde parou depois de andar 5 tiles), `npc` e `object` (a opção de menu clicada),
`dialogue` (a opção escolhida, clicada ou pela tecla), `shop_buy` (o que entrou no inventário com a
loja aberta), `bank_open`, `bank_withdraw` e `bank_deposit` (a diferença do inventário e do equipamento
com o banco aberto), `item_use` e `item_op`. Os eventos vão no campo novo `data.recording` do ingest
(flag `recording` no manifest), ficam em `recorded_events` (migration 0017; o ingest é o único escritor,
cada `(recording, seq)` uma vez só) e o dono os lê em `GET /api/account/recording` e apaga em
`DELETE /api/account/recording` (`chronicle recording`). Contrato e detalhes no doc 06 (adendo de
2026-11-13 sobre a gravação) e no doc 04, seção 6.4. Decisões conservadoras pendentes do dono na
seção 14 (E11 a E13).

**Como implementado (fase 2, as propostas, 2026-11-13).** `internal/core/recording/propose` (puro, testado
por tabela) transforma um trecho gravado em tarefas propostas para um passo, na ordem em que aconteceram:

| Eventos | Tarefa proposta |
|---|---|
| a interação acontece num tile dentro da área de um local novo | `travel(location)`; com `via` quando um `item_op` antes dela levou o personagem a 30 tiles ou mais (um teleporte) |
| `npc` com Talk-to e as opções de diálogo que seguem | `talk(npc, options)`, `until: goal_met` quando o passo conclui algo |
| `npc` com Trade e as compras que seguem | um `buy(shop, item, qty)` por item: a loja é a única, entre as que o lojista é dono, que vende o item |
| `object` (que não abre o banco) | `interact(object, action)` |
| abertura do banco (pelo NPC ou objeto antes dela), saques e depósitos | `bank(items)` com o que foi sacado e não devolvido |
| `item_use` | `manual` com a nota "Usar X em Y" (não há tipo de tarefa para isso ainda) |

Os ids de jogo viram refs pelos catálogos da fase 1 (`NPCsByGameID`, `ObjectsByGameID`, `ShopsOwnedBy`,
`LocationsAt`). Um id em várias páginas, nenhuma loja certa ou um tile em várias áreas (ou em nenhuma)
vão para `issues`, nunca para um palpite. Toda proposta tem `confidence: medium`, os eventos que a
sustentam e o que o verificador diz dela (`problems`, por exemplo um `talk` sem objetivo para terminar).

No `content mcp`: `propose_tasks` lê o arquivo de `chronicle recording -o` (o `content mcp` continua sem
banco e sem rede), recorta o trecho do passo e devolve as propostas no formato do `submit_tasks`; a sessão
de extração (skill `annotate-guide@6`) pergunta ao dono sobre cada uma (AskUserQuestion) e envia só o
confirmado, com `confirmed_by: owner`. O `submit_tasks` copia os eventos do arquivo pelos `seq`, nunca
do agente, e passa pelo verificador da seção 7. O `submit` também aceita `tasks` e, sem elas, mantém as
tarefas do passo.

## 12. Modelo de dados proposto

Novo, alterado e mantido em relação às migrations 0001 a 0006. Formato final de cada tabela na
migration de cada fase.

| Tabela | Estado | Chave e campos principais |
|---|---|---|
| `guides` | nova (migration 0009) | `id`, título, atribuição; preenchida no boot; FK de `accounts`, `step_marks`, `route_cursor`, eventos |
| `observations` | nova | `(account_id, facet, key, source)`, `value`, `observed_at` limitado, `snapshot_id` |
| `player_state` | muda: projeção | `(account_id, facet, key)`, `value` da junção |
| `mark_commands` | nova, só adição (migration 0009) | `seq`, `id`, `account_id`, `guide_id`, `kind`, `actor`, `via`, `step_id`, `revokes_id`, `effects`, `at` |
| `step_marks` | muda: projeção | ganha `command_id` |
| `objective_progress` | mantém, com mais restrições | ganha `actor`, `run_id`; CHECK de evidência não vazia; trava no banco |
| `clients` | nova | `id`, máquina, perfil do Microbot |
| `accounts` | muda | `guide_id` com FK, `client_id`, `active_plan_id` |
| `plans`, `plan_items` | novas | `kind ∈ {route, goal, custom}`; item `step`, `recipe` ou `task` |
| `routine_runs`, `run_events` | novas (doc 08) | execução, `seq`, `kind`, `detail` |
| `outbox` | nova | `id`, `account_id`, `topic`, `payload` |

No conteúdo: `tasks[]` no schema das anotações; `content/recipes/` com schema próprio; entidades `npc`,
`shop`, `object` e área em `location`. Feito na fase 1 (seções 7 e 8).

Contrato API ↔ web (R11): tipos de resposta exportados num pacote `internal/httpapi/apitypes`, TypeScript
gerado pelo tygo em `web/src/lib/api/gen/`, `just gen-types` e `git diff --exit-code` no CI; fixtures JSON
por endpoint validadas com Zod. A web deixa de reimplementar regras do servidor: a API manda `done` e
`before_count` prontos.

Como implementado até agora (fatia f, 2026-11-05): as **fixtures** existem, sem Zod.
`TestContractPostgres` (`internal/httpapi`) sobe a API real com Postgres e o conteúdo real, faz um push,
marcas, uma meta e um sync, e grava em `web/src/lib/api/contract/` uma resposta real por rota que a web
lê, como `export const fixture = {…} satisfies <tipo>`. O `satisfies` faz o `svelte-check` (já no CI)
recusar campo obrigatório faltando, `null` onde o tipo não aceita, valor fora de uma união e campo que o
Go manda e o tipo não declara; o teste Go recusa a mudança de forma de uma resposta até alguém rodar
`-update` e o `pnpm check`. A primeira rodada achou 16 divergências no `types.ts`, todas corrigidas. A
**geração com tygo** ficou para decisão do dono: os tipos de resposta da API são não exportados no
`internal/httpapi`, o struct embutido pede `tstype:",extends"`, cada tipo de outro pacote pede um
mapeamento manual e os nomes do TS e do Go divergem; as fixtures já cobrem a detecção de divergência.

**Adiado pelo dono (2026-11-05), com OpenAPI como candidato no lugar do tygo.** Motivo: o tygo só
gera tipos para a web, e o contrato de ingest e as rotas `/api/plugin` têm um segundo consumidor, o
plugin em Java, que hoje só tem o espelho de `docs/arquitetura/types.ts`. Caminho proposto ao retomar:
(1) `openapi.yaml` escrito primeiro para `/api/ingest`, `/api/ingest/manifest` e `/api/plugin/*`, com
o cliente Java gerado dele; (2) depois as rotas da web, com tipos de `openapi-typescript`; (3) as
fixtures viram teste de conformidade, com cada resposta real validada contra o spec no Go
(`kin-openapi`).

## 13. Fases

| Fase | Entrega | Pronto quando |
|---|---|---|
| 0 | Restrições e locks (R1 a R4, R6, R7), `observations` e junção pura, `owner_commands`, autor na evidência, `guides`, `progress rebuild --check`; correções da web (R9, R10); contrato gerado (R11) | `rebuild --check` sem diferença no CI e no banco do zaros; teste de concorrência ingest + sync + marca no Postgres |
| 1 | Entidades `npc`, `shop`, `object`; áreas em `location`; schema de `tasks[]` e de receitas; verificador | `content validate` cobre tarefas e receitas |
| 2 | Gravação por demonstração e modo assistido no plugin | um trecho do Oziris 1.1 com tarefas vindas da gravação, confirmadas pelo dono |
| 3 | Executor de `travel`, `bank`, `buy`; `plans` e `plan_items`; comando de tarefa no agent | um trecho de logística executado com término provado pelo ingest (código pronto em 2026-11-13; falta o dono no jogo) |
| 4 | `train` por delegação; rotinas do doc 08 com `wait_until` por chave de objetivo | um treino até `skill:x>=N` sem intervenção (código pronto em 2026-11-13; falta o dono no jogo) |
| 5 | Receitas de quest, planos customizados na UI, vários clientes no agent | por entrega |

A fase 0 não dependia de nenhuma decisão de automação e já está feita. Com a seção 14 fechada
(2026-11-13), as fases 1 a 5 entraram na sequência do doc 04, seção 9, como item 21. A fase 1 foi
começada e terminada em 2026-11-13 (seções 7 e 8): `content validate` cobre tarefas e receitas.

**Fase 2: o código está pronto (2026-11-13); falta o dono.** Feito: a gravação no Chronicle Link 1.13.0
e no ingest (seção 11), as tarefas propostas da gravação com o `propose_tasks` e o `submit_tasks`
(seções 7 e 11) e o modo assistido no painel do 1.14.0 (seção 10). Para terminar a fase, o dono:

1. instala o Chronicle Link 1.14 e liga "Record demonstration" (e "Send position", para o modo
   assistido avaliar os `travel`);
2. joga um trecho do Oziris 1.1 (por exemplo do 1.1.1 ao 1.1.20);
3. exporta a gravação: `chronicle recording --since 3h -o /tmp/gravacao.json`;
4. abre uma sessão de extração (skill `annotate-guide@6`): para cada passo do trecho, o agente chama o
   `propose_tasks` com o arquivo, mostra as propostas e as `issues`, e o dono confirma, corrige ou
   descarta cada uma; o `submit_tasks` grava o confirmado com `confirmed_by: owner`;
5. confere no painel do plugin a lista de tarefas dos passos que ganharam tarefas, e commita os
   sidecars depois do `content validate`.

O que não foi conferido no jogo está no doc 06 (adendos da gravação e do modo assistido). A fase 3 (o
executor de `travel`, `bank` e `buy`) parte das tarefas assim confirmadas.

**Fase 3: o código está pronto (2026-11-13); falta o dono.** Feito, em três PRs empilhados:

1. os planos (`plans`, `plan_items`, a próxima tarefa do plano ativo, seção 9);
2. o comando `task_run` no agent, com `POST` e `GET /api/microbot/task` (a prova do término pelo
   ingest) e a CLI `chronicle task` (seção 10 e doc 08);
3. o executor no Chronicle Link 1.15.0 (seção 10 e doc 06).

"Pronto quando um trecho de logística executado com término provado pelo ingest": isso só o dono faz,
no jogo e olhando. Como ainda não há tarefa real no conteúdo, o trecho sai de um plano customizado
(`chronicle plans create -f`) ou de tarefas confirmadas na fase 2. O dono:

1. instala o Chronicle Link 1.15 no cliente de dev (`mb-install chroniclelink`), reinicia o cliente e
   o `just agent`, liga "Send position" no plugin e confere "Avoid wilderness" no webwalker;
2. monta um plano de logística perto de onde o personagem está e passa a segui-lo:
   `chronicle plans create -f logistica.json` e `chronicle plans use <id>`. Por exemplo, um `travel`
   até um local com área, um `bank` com itens que o banco tem e um `buy` numa loja ao lado;
3. confere a próxima tarefa (`chronicle task`) e roda uma por vez, olhando pelo VNC:
   `chronicle task run --yes --wait 10m`. O run anda e o ingest prova o término ("Término provado pelo
   ingest"); o plano passa à próxima;
4. confere no Cockpit a trilha dos runs e, na auditoria, o `task_run`; ao terminar, volta à rota com
   `chronicle plans use route`.

Uma falha (sem banco por perto, item faltando, loja sem estoque, o webwalker parando antes) deixa a
tarefa onde parou, com o motivo na trilha: é o dono quem decide o que fazer.

**Fase 4: o código está pronto (2026-11-13); falta o dono.** Feito, em dois PRs empilhados (seção 10,
"Como implementado (fase 4)", e doc 08):

1. o `wait_until` por chave de objetivo nos runs e nas rotinas, com o vigia no servidor e o comando
   `run_until`;
2. o `train` por delegação a um plugin, no conteúdo, no `execspec`, no plano e no `task_run`.

Testado com o jogo, a ponte e o cliente falsos, e de ponta a ponta no Postgres: um plano com um
`train` até `skill:fishing>=76`, o `POST /api/microbot/task` liga o plugin com as opções dele, o ingest
no 75 não muda nada, o ingest no 76 faz o servidor avisar o agent, que desliga o plugin, e o `GET
/api/microbot/task` dá `proven`. Nada rodou no jogo. "Pronto quando um treino até `skill:x>=N` sem
intervenção" é do dono, olhando a primeira vez:

1. atualiza o `chronicle` da máquina do jogo (o `just agent` do checkout) e o do servidor: os dois
   precisam do `run_until`; o Chronicle Link continua o 1.15 (o treino não muda o plugin, só precisa
   do rastreamento ligado);
2. escolhe um treino curto e seguro, de preferência sem combate (por exemplo, pescar até o próximo
   nível) e um plugin que já usa para isso, instalado no cliente (pelo perfil da conta, pelo modelo
   da frota ou pelo Hub) e desligado; anota as opções que ele precisa (grupo e chave estão no
   formulário de Plugins do Cockpit);
3. monta um plano com a tarefa e passa a segui-lo:

   ```
   {"kind": "custom", "title": "Treino", "items": [{"kind": "task", "task": {
     "type": "train", "skill": "skill:fishing", "plugin": "<classe do plugin>",
     "config": [{"group": "<grupo>", "key": "<chave>", "value": "<valor>"}],
     "until": {"objective": "skill:fishing>=<N>"}}}]}
   ```

   `chronicle plans create -f treino.json` e `chronicle plans use <id>`;
4. leva o personagem até o lugar do treino (o `train` não anda até lá: um `travel` antes, no mesmo
   plano, faz isso), confere `chronicle task` e roda `chronicle task run --yes --wait 3h`; olha pelo
   VNC que o plugin ligou e treina;
5. sem mexer em nada: quando o nível sobe, o plugin manda a captura, o servidor prova o nível e o
   agent desliga o plugin; a CLI mostra "Término provado pelo ingest"; na auditoria, o `task_run` e
   o `run_until` (ator `wait_until`, origem `server`);
6. volta à rota com `chronicle plans use route`.

O `wait_until` numa rotina também fica para o dono provar: um "Fazer agora" com `until` pelo `POST
/api/microbot/run?account=` (a web ainda não tem o campo; pede o canvas).

## 14. Decisões em aberto (dono)

| # | Pergunta | Recomendação |
|---|---|---|
| E1 | Fatos por fonte com junção pelo maior valor para os monotônicos | **decidida (2026-11-05): sim** |
| E2 | `owner_commands` como log e `step_marks` como projeção | **decidida (2026-11-05): sim** |
| E3 | `progress rebuild --check` no CI, no boot ou só sob demanda | **decidida (2026-11-05): CI e comando; no boot só aviso, sem bloquear** |
| E4 | Contrato gerado com tygo + fixtures Zod | **decidida (2026-11-05): sim** |
| E5 | Lista inicial de primitivas (`travel`, `bank`, `buy`, `talk`, `interact`, `train`, `manual`) | **decidida (2026-11-13): sim; `quest` depois** |
| E6 | Receitas em `content/recipes/` (git) | **decidida (2026-11-13): sim** |
| E7 | Planos customizados no banco, exportáveis para `content/routes/` | **decidida (2026-11-13): sim** |
| E8 | Gravação por demonstração no plugin (o plugin passa a registrar interações, ainda sem agir) | **decidida (2026-11-13): sim, opt-in** |
| E9 | Política de risco: em HCIM, nada com risco de morte roda sem supervisão | **decidida (2026-11-13): sim** |
| E10 | Importar diálogos e etapas de quest do Quest Helper em vez de extrair do texto | **decidida (2026-11-13): avaliar na fase 5. Avaliada (2026-11-13, apêndice A):** não importar os dados (são código Java, não dados); executar por delegação (E19) |
| E11 | O que a gravação registra | **em aberto; implementado o conservador (fase 2):** só o que o dono faz (NPCs e objetos clicados, opções de diálogo, compras, banco, itens usados e opções de item, posição grosseira); nunca chat, outros jogadores, magias, Drop/Examine ou o caminho tile a tile |
| E12 | Retenção da gravação no servidor | **em aberto; implementado o conservador:** fica até o dono apagar (`DELETE /api/account/recording`, `chronicle recording delete --yes`), sem expiração automática; o snapshot bruto não guarda a gravação |
| E13 | A gravação vai para o conteúdo como evidência | **em aberto; implementado o conservador:** só os eventos que sustentam cada tarefa confirmada, sem conta, RSN ou outros dados do personagem (`submit_tasks` copia só os `seq` pedidos) |
| E14 | `travel` com `via` (teleporte por unlock ou item) no executor | **em aberto; implementado o conservador (fase 3):** recusado ("faça à mão"); a alternativa é deixar o webwalker escolher o caminho, com os teleportes que a configuração dele permitir |
| E15 | O que o `bank` faz com o que não é do setup | **em aberto; implementado (fase 3):** deposita antes de sacar (nada se perde: vai para o banco, e o inventário precisa de espaço); a alternativa é só sacar e falhar sem espaço |
| E16 | `train` com o plugin já ligado | **em aberto; implementado o conservador (fase 4):** recusado ("pare-o antes"), porque o run liga e desliga o plugin e grava as opções antes de ligar; a alternativa é adotar o plugin ligado, sem gravar opções |
| E17 | As opções que o `train` gravou no plugin | **em aberto; implementado (fase 4):** ficam como gravadas depois do treino (são do plugin, e o próximo `train` grava as dele de novo); a alternativa é guardar os valores de antes e restaurá-los |
| E18 | De onde vem o plugin de um `train` de passo | **em aberto; implementado o conservador (fase 4):** de uma receita da chave (`content/recipes/`) ou de uma tarefa avulsa num plano; a extração do texto (`submit_tasks`) não aceita `plugin` nem `config`, porque o texto do guia não diz qual plugin usar; a alternativa é o dono confirmá-lo na sessão de extração, como uma inferência |
| E19 | Primitiva `quest` por delegação ao Quest Helper que o cliente do Microbot traz (apêndice A, A.6) | **em aberto; nada implementado.** Recomendação: sim, como o `train` (seleciona a quest pelo Agent Server, acompanha, e o servidor prova `quest:x@finished` pelo ingest). Em HCIM, só supervisionada; "Obtain Missing Items" em `No` numa conta ironman. Começar por uma quest curta e sem luta, com o dono olhando |
| E20 | Exportador dos passos do Quest Helper como fonte de propostas de receita (apêndice A, A.5, caminho 2) | **em aberto; nada implementado.** Recomendação: não agora. Só se a extração do texto e a gravação não bastarem para as receitas de quest |

## 15. Impacto nos outros docs

- Doc 01: decisão 32 (este modelo) aberta até as respostas de E1 a E10.
- Doc 04, seção 9: a fase 0 entra como item próprio da sequência quando aprovada.
- Doc 08: rotinas usam `wait_until` por chave de objetivo; o agent ganha o comando de tarefa.
- Doc 09: seções 3.1 (tipos `npc`, `shop`, `object`) e 5 (tabelas) recebem adendos apontando para cá.

## Apêndice A. E10: o Quest Helper como fonte de quests (avaliação, 2026-11-13)

A pergunta da E10 era: importar diálogos e etapas de quest do Quest Helper em vez de extrair do texto
dos guias? Foi respondida só por leitura. Nada do Quest Helper (código ou dados) entrou no repositório.

**Fontes lidas:**

- o repositório `Zoinkwiz/quest-helper` no commit `b9b93706` (2026-11-03, versão 4.18.0);
- a cópia que o cliente do Microbot fixado (2.6.52, `microbot/microbot.json`) traz em
  `runelite-client/.../microbot/questhelper/`, lida no `microbot/vendor` (sincronizada com o upstream
  `5539626` de 2025-08-29, conforme `.quest-helper-sync`);
- o `QuestHelperHandler` do Agent Server do mesmo cliente.

As contagens abaixo são buscas nas fontes desse commit.

### A.1 Licença

- O repositório é **BSD 2-Clause**: `LICENSE`, "Copyright (c) 2020, Zoinkwiz".
  - 676 dos 714 arquivos Java têm o cabeçalho BSD-2, com 52 titulares diferentes; os outros ficam sob
    o `LICENSE`.
  - Não há outra licença no repositório: os recursos são 23 PNGs da interface, e o código vindo do
    RuneLite também é BSD-2.
  - O Plugin Hub pede BSD-2 de todo plugin. A cópia no Microbot também é BSD-2.
- **Dados derivados num repositório privado:** permitido. O BSD-2 deixa usar e modificar, e uma cópia
  privada não é redistribuição. Se o repositório for publicado, ele leva o aviso BSD-2 e o crédito
  "Quest Helper, Zoinkwiz e contribuidores", e as telas que mostrarem esses passos levam a atribuição,
  como a regra 6 faz com os guias.
  - O texto dos passos é dos contribuidores.
  - Ids, coordenadas e falas de diálogo são conteúdo do jogo (Jagex), como nos guias.
- **Código:** o BSD-2 permite redistribuir com o aviso, sem copyleft.
- O limite real não é a licença: é o risco de banimento de automatizar, aceito pelo dono (doc 01,
  decisão 9).
- Esta leitura não é um parecer jurídico.

### A.2 Tamanho e manutenção

- **464 arquivos de helpers**, cerca de 167 mil linhas:
  - 185 pacotes de quest com 334 arquivos;
  - 50 arquivos de achievement diaries;
  - 30 de miniquests;
  - o resto em atividades, skills e diversos.

  O registro (`QuestHelperQuest`) tem 276 entradas.
- **Ativo:** 228 commits nos últimos 12 meses, cerca de 307 contribuidores.
  - Os ids vêm das constantes `gameval` do RuneLite (`NpcID.COOK`, `ObjectID.…`, `ItemID.…`). Uma
    renomeação no RuneLite quebra o build, e o upstream corrige; o último commit lido foi uma dessas
    correções.
  - A ordem das quests (inclusive a "Ironman Optimal") é regenerada da wiki por um CI noturno.

### A.3 Como uma quest é codificada

**É código Java, não dado.** Não há export em JSON ou YAML, nem um módulo de dados. Não achamos
ferramenta da comunidade que exporte os passos.

- **O quadro geral:**
  - `BasicQuestHelper.loadSteps()` devolve um `Map<Integer, QuestStep>` indexado pelo valor da
    varp ou do varbit da quest (`QuestVarPlayer`, `QuestVarbits`): em jogo, o helper lê a variável e
    mostra o passo daquele valor.
  - Cada valor costuma ser uma `ConditionalStep`, uma árvore de pares (condição, passo) em que a
    primeira condição verdadeira vence.
  - As condições olham o inventário, zonas, varbits e o que já apareceu em diálogos e no chat.
  - Os valores das chaves são achados testando no jogo. Um comentário na Waterfall Quest diz:
    "7 didn't occur during testing".
- **Tipos de passo** (em todos os helpers):

  | Tipo | Quantos |
  |---|---|
  | `ObjectStep` | 5.367 |
  | `NpcStep` | 3.439 |
  | `ConditionalStep` | 2.703 |
  | `DetailedQuestStep` | 1.107 |
  | `PuzzleWrapperStep` | 339 |
  | `ItemStep` | 201 |
  | `WidgetStep` | 154 |
  | `DigStep`, `TileStep`, `EmoteStep` | poucos |

  Os passos levam o id do jogo e um `WorldPoint`; há 14.151 `WorldPoint` só nas quests.
- **Diálogo:** `addDialogStep(s)` e variantes, cerca de 2.350 chamadas nas quests. É um **conjunto
  não ordenado** de opções a destacar quando a caixa de opções aparece, sem as falas do NPC. Exemplo
  do Cook's Assistant:

  ```java
  finishQuest = new NpcStep(this, NpcID.COOK, new WorldPoint(3206, 3214, 0), "Give the Cook ...", egg, milk, flour);
  finishQuest.addDialogSteps("What's wrong?", "Can I help?", "Yes.");
  ```
- **Ação de menu:** o `ObjectStep` não guarda a ação de menu, só o id e o texto. O bot do Microbot a
  deduz casando o texto do passo com as ações do objeto.
- **Requisitos tipados** (nas quests):

  | Requisito | Quantos |
  |---|---|
  | `ItemRequirement` | 5.672 |
  | `VarbitRequirement` | 2.372 |
  | `ZoneRequirement` | 2.059 |
  | `SkillRequirement` | 1.158 |
  | `QuestRequirement` | 468 |

  Há também condições que só existem como evento em jogo: `DialogRequirement`,
  `ChatMessageRequirement`, `WidgetTextRequirement` e `ManualRequirement`.

### A.4 O que casa com o nosso modelo

| Quest Helper | Chronicle (seções 7 e 8) | Casa? |
|---|---|---|
| `NpcStep` com id e opções de diálogo | `talk(npc, options)`, com o id virando ref pelo `NPCsByGameID` (fase 1) | em parte: as opções são um conjunto e o nosso `options` é uma ordem; o término de um passo do meio é "a varp da quest andou", que o ingest não observa |
| `ObjectStep` com id | `interact(object, action)`, com o id pelo `ObjectsByGameID` | em parte: falta a `action`, que viria do texto ou da gravação |
| `ItemRequirement`, `ItemCollections` | `requires`, `bank(items)` e os ids aceitos | sim |
| `SkillRequirement`, `QuestRequirement` | `skill:x>=N`, `quest:x@finished` (catálogo de quests da wiki) | sim |
| `WorldPoint`, `ZoneRequirement` | `travel(location)` | não: um tile qualquer não é um local do catálogo, e não há "andar até o tile" |
| `ConditionalStep`, varbits, condições de evento | nada | não: as nossas tarefas são uma lista com término por chave de objetivo ou estado, e o ingest só vê a quest como não começada, em andamento ou terminada |
| `PuzzleStep`, passos próprios | `manual` | só como "faça à mão" |

O Quest Helper descreve uma **máquina de estados** sobre a variável da quest. As nossas receitas são
**listas lineares** de tarefas tipadas com término provado pelo ingest (princípios 5 e 7). Traduzir
uma na outra perde os ramos, e os passos do meio ficam sem término avaliável.

### A.5 Três caminhos

1. **Parser estático do Java, em Go: não recomendado.**
   - Só 106 das 185 quests usam apenas os blocos básicos num arquivo só, e só 19 são quase lineares
     (até duas `ConditionalStep`).
   - 52 têm vários arquivos, 48 têm classes de passo próprias, 44 têm quebra-cabeças e 46 reagem a
     eventos (`@Subscribe`).
   - O parser teria de resolver campos entre métodos, combinadores (`and`, `or`, `nor`,
     `Conditions`), `.copy()`, `setText`, concatenação de texto e os nomes `gameval` na versão exata
     do RuneLite.
   - Ele quebraria a cada renomeação, e o resultado ainda seria a árvore, não uma receita.
2. **Exportador em Java no build: possível, como fonte de propostas.** Uma ferramenta no projeto
   Gradle de `microbot/` monta cada helper com um `Client` falso, como o `QuestHelperTest` do próprio
   repositório já faz, e percorre os objetos criados.
   - Ela emitiria JSON com: o mapa varp → passo, a árvore, os tipos, os ids numéricos, os
     `WorldPoint`, as opções de diálogo e os requisitos tipados.
   - Passos próprios, quebra-cabeças e condições de evento sairiam opacos.
   - O JSON seria uma **proposta** de receita, como a gravação da fase 2, verificada e confirmada
     pelo dono (regra 9), nunca uma receita pronta.
   - Esforço: 3 a 5 dias para o exportador e a conversão em propostas, mais a curadoria quest a
     quest com o dono.
3. **Delegação em execução ao Quest Helper do Microbot: recomendado para executar.** O cliente
   fixado já traz o Quest Helper (183 pacotes de quest) e, nele:
   - o `QuestScript` (cerca de 1.750 linhas), um bot que segue o passo ativo do helper: anda com o
     `Rs2Walker`, fala com NPCs, escolhe as opções destacadas, interage com objetos, cava e resolve
     alguns quebra-cabeças;
   - o Agent Server com `/quest-helper/list`, `/states`, `/status` (quest, se terminou, texto e classe
     do passo ativo), `/start` (seleciona a quest e liga "Enable QuestHelper") e `/stop`.

   Uma quest vira então uma tarefa de **delegação**, como o `train` da fase 4: o agent seleciona a
   quest, acompanha o passo na trilha, e o run termina só quando o servidor prova
   `quest:<x>@finished` pelo ingest (`wait_until`). Não é preciso copiar dados nem código para o
   repositório, nem manter uma cópia da lógica em dia: ela vem com o cliente.

### A.6 O que a primitiva `quest` precisaria (caminho 3)

- **Conteúdo:**
  - `quest` passa a ser primitiva (hoje o verificador a recusa, E5), com a ref da quest do catálogo e
    o término padrão `quest:<x>@finished`;
  - ela vem de uma receita da chave (`content/recipes/quest-<x>-finished.json`) com os riscos da
    variante (E9: quase toda quest com luta tem `combat`), confirmada pelo dono, como o plugin de um
    `train` (E18).
- **`execspec` e plano:**
  - o nome da quest como o Quest Helper a conhece: o nome do RuneLite, que o ingest já usa
    (`data.quest`), conferido no `/quest-helper/list` do cliente;
  - os requisitos do catálogo provados antes de rodar, porque o plano já insere a receita do que
    falta (seção 9);
  - em HCIM, só supervisionado.
- **Agent** (`task_run` de um `quest`), em ordem:
  1. ponto seguro e login;
  2. o Quest Helper ativo no cliente (`/quest-helper/status`);
  3. "Obtain Missing Items" em `No` numa conta ironman. O padrão é `Ask`, que pergunta numa janela
     do cliente e prende um run sem ninguém olhando; com `Yes`, o bot iria ao banco e ao Grand
     Exchange;
  4. `/quest-helper/start`, depois o `status` na trilha (progresso, nunca prova);
  5. `/quest-helper/stop` na prova, no timeout, na falha ou no cancelamento.
- **Servidor:** o `POST /api/microbot/task` recusa com 409 um agent sem o recurso e uma quest que o
  cliente não lista; a prova é a de sempre.
- **Esforço:** 2 a 3 dias de código e testes com um Quest Helper falso, sem migration. Depois, uma
  quest curta e sem luta no jogo, com o dono olhando (por exemplo Cook's Assistant ou Sheep Shearer,
  com os itens no banco).

### A.7 Riscos

- **Banimento:** um bot de quest é mais visível que um treino.
- **Morte:** quests com luta; a E9 vale.
- **Conta ironman:** o bot completa itens pelo Grand Exchange, então a opção fica em `No` e os
  itens vêm do banco, por um `bank` antes.
- **Confiabilidade desigual:**
  - o `QuestScript` tem casos especiais escritos à mão (Cook's Assistant, Imp Catcher, Pirate's
    Treasure) e só alguns quebra-cabeças;
  - quests longas devem travar no meio. O run para, e a quest volta ao dono (`manual`, seção 11).
- **Defasagem:** a cópia do Microbot está sincronizada até 2025-08-29, e quests mais novas que isso
  faltam. Um `mb-bump` pode mudar o comportamento, e os endpoints existem desde 2026-04.
- **Nada disso foi testado no jogo.**

### A.8 Recomendação

1. **Para executar quests: o caminho 3** (E19, em aberto para o dono): a primitiva `quest` por
   delegação, reaproveitando o `task_run` e o `wait_until` do `train`. São de 2 a 3 dias, sem
   migration, e o término continua sendo a prova pelo ingest (princípio 7).
2. **Para o conteúdo das receitas:** não importar.
   - As receitas de quest continuam vindo do texto dos guias e da gravação, com o dono (regra 9).
   - O Quest Helper serve de referência humana nessas sessões.
   - O exportador (caminho 2, E20) fica para quando a extração e a gravação não bastarem.
3. **O parser estático (caminho 1), não.**
4. **Nada do Quest Helper no repositório.** A delegação usa o que o cliente já traz. Se um dia algum
   dado derivado entrar, ele leva o aviso BSD-2 e o crédito.
