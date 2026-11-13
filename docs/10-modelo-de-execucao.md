# 10 — Modelo de execução: fatos, eventos, tarefas, receitas e planos

**Estado: proposta (2026-11-05). E1 a E4 decididas pelo dono em 2026-11-05 e E5 a E10 em 2026-11-13, do
jeito recomendado (seção 14). As fases 0 (doc 04, seção 9, item 17) e 1 (item 21, 2026-11-13) estão
feitas.** A fase 1 entregou as entidades `npc`, `shop` e `object`, as áreas dos locais, o schema de
`tasks[]` e de receitas e o verificador (seções 7 e 8, "Como implementado"), sem nenhuma tarefa ou
receita real no conteúdo: elas vêm da extração com o dono e da gravação (fase 2). O resto deste doc
não está implementado. Ele estende o doc 09 (o modelo de objetivos continua valendo) e o doc 08 (o controle do
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
| `quest` | `quest` | `quest:x@finished` | receita própria ou dados do Quest Helper |
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
- **Ferramental**: o `revise_entity` do `content mcp` acha e renomeia os refs das tarefas dos sidecars. O
  `submit` ainda não aceita tarefas; entra com a extração delas na fase 2.

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
Helper em vez de extrair do texto) e combate real, que começa como `manual` ou delegado.

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
| 3 | Executor de `travel`, `bank`, `buy`; `plans` e `plan_items`; comando de tarefa no agent | um trecho de logística executado com término provado pelo ingest |
| 4 | `train` por delegação; rotinas do doc 08 com `wait_until` por chave de objetivo | um treino até `skill:x>=N` sem intervenção |
| 5 | Receitas de quest, planos customizados na UI, vários clientes no agent | por entrega |

A fase 0 não dependia de nenhuma decisão de automação e já está feita. Com a seção 14 fechada
(2026-11-13), as fases 1 a 5 entraram na sequência do doc 04, seção 9, como item 21. A fase 1 foi
começada e terminada em 2026-11-13 (seções 7 e 8): `content validate` cobre tarefas e receitas. A
próxima é a 2, a gravação por demonstração e o modo assistido, que traz as primeiras tarefas reais.

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
| E10 | Importar diálogos e etapas de quest do Quest Helper em vez de extrair do texto | **decidida (2026-11-13): avaliar na fase 5** |

## 15. Impacto nos outros docs

- Doc 01: decisão 32 (este modelo) aberta até as respostas de E1 a E10.
- Doc 04, seção 9: a fase 0 entra como item próprio da sequência quando aprovada.
- Doc 08: rotinas usam `wait_until` por chave de objetivo; o agent ganha o comando de tarefa.
- Doc 09: seções 3.1 (tipos `npc`, `shop`, `object`) e 5 (tabelas) recebem adendos apontando para cá.
