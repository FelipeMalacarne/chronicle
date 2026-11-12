# 10 — Modelo de execução: fatos, eventos, tarefas, receitas e planos

**Estado: proposta (2026-11-05). E1 a E4 decididas pelo dono em 2026-11-05 (seção 14); a fase 0 está na
sequência do doc 04, seção 9 (item 17). E5 a E10 continuam abertas.** Nada deste doc está implementado. Ele estende o doc 09 (o modelo de objetivos continua valendo) e o doc 08 (o controle do
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
`shop`, `object` e área em `location`.

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

A fase 0 não depende de nenhuma decisão de automação e pode começar antes; as fases 1 em diante entram na
sequência do doc 04, seção 9, quando o dono fechar a seção 14.

## 14. Decisões em aberto (dono)

| # | Pergunta | Recomendação |
|---|---|---|
| E1 | Fatos por fonte com junção pelo maior valor para os monotônicos | **decidida (2026-11-05): sim** |
| E2 | `owner_commands` como log e `step_marks` como projeção | **decidida (2026-11-05): sim** |
| E3 | `progress rebuild --check` no CI, no boot ou só sob demanda | **decidida (2026-11-05): CI e comando; no boot só aviso, sem bloquear** |
| E4 | Contrato gerado com tygo + fixtures Zod | **decidida (2026-11-05): sim** |
| E5 | Lista inicial de primitivas (`travel`, `bank`, `buy`, `talk`, `interact`, `train`, `manual`) | sim; `quest` depois |
| E6 | Receitas em `content/recipes/` (git) | sim |
| E7 | Planos customizados no banco, exportáveis para `content/routes/` | sim |
| E8 | Gravação por demonstração no plugin (o plugin passa a registrar interações, ainda sem agir) | sim, opt-in |
| E9 | Política de risco: em HCIM, nada com risco de morte roda sem supervisão | sim |
| E10 | Importar diálogos e etapas de quest do Quest Helper em vez de extrair do texto | avaliar na fase 5 |

## 15. Impacto nos outros docs

- Doc 01: decisão 32 (este modelo) aberta até as respostas de E1 a E10.
- Doc 04, seção 9: a fase 0 entra como item próprio da sequência quando aprovada.
- Doc 08: rotinas usam `wait_until` por chave de objetivo; o agent ganha o comando de tarefa.
- Doc 09: seções 3.1 (tipos `npc`, `shop`, `object`) e 5 (tabelas) recebem adendos apontando para cá.
