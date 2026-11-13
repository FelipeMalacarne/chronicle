# Controle do Microbot pelo planner

Data: 2026-11-04

## Objetivo

Usar e controlar o cliente Microbot pelo webapp, não só um plugin específico:

- ver o jogo ao vivo no navegador e intervir com mouse e teclado quando algo trava;
- alternar para **só dashboard** (estado, mapa, screenshot periódico), que é leve e funciona no celular;
- abrir e fechar o cliente, logar e deslogar;
- instalar plugins do Plugin Hub, iniciar, parar e acompanhar qualquer plugin, e ajustar a configuração;
- montar **rotinas** (ex.: logar, rodar o Birdhouse Runner até terminar, deslogar) e agendá-las.

## Mudança de decisão em relação ao doc 06

O doc 06 definiu o plugin do planner como **somente leitura**, para não misturar o risco de ban dos
scripts com a conta ironman. Este doc inverte isso **apenas no módulo de controle**, por decisão do
dono em 2026-11-04, com o risco aceito conscientemente. Para conter o impacto:

- O módulo de controle é separado do tracking e vem **desligado por padrão** (`MICROBOT_CONTROL=off`).
  Com ele desligado, o planner continua exatamente como no doc 06.
- O tracking continua vindo do plugin somente leitura (ingest). O controle não escreve progresso.
- Toda ação de controle fica registrada (quem, quando, o quê, resultado) para auditoria.

## O que já existe (o harness em `microbot/`)

O harness de desenvolvimento de plugins já resolve o lado do cliente:

| Peça | O que faz |
|---|---|
| `mb-client start --headless` | Sobe o cliente fixado (versão e hash em `microbot/microbot.json`) num display próprio `Xvfb :99`, com home isolada (`~/.local/share/microbot-dev`), HWID agent e Agent Server ligado. GPU e 117 HD ficam desligados (Xvfb não tem GPU). |
| VNC | `x11vnc` só em `127.0.0.1:5999`, sem senha; quem expõe (o planner) autentica. |
| Agent Server | API HTTP do próprio Microbot num socket Unix (`~/.local/share/microbot-dev/.runelite/agent/agent.sock`), com token em `.agent-token`. |
| `mb-account` | Escolhe o personagem da Jagex Account com que o cliente abre. |

Por que headless: uma janela do cliente escondida ou minimizada no desktop cai para cerca de 1 quadro
por segundo, e tudo (caminhar, clicar, esperar) fica lento. Medido com o personagem logado: chamadas
ao Agent Server em ~1000 ms com a janela escondida contra ~18 ms no Xvfb.

Endpoints do Agent Server que o módulo usa (referência completa em
`microbot/vendor/microbot/docs/AGENT_SERVER.md`):

| Necessidade | Endpoint |
|---|---|
| Estado do jogo e do jogador | `GET /state` |
| Login e logout | `GET/POST /login`, `POST /logout` |
| Plugins do Hub | `GET /plugins`, `POST /plugins/install`, `POST /plugins/remove` |
| Ciclo de vida de scripts | `GET /scripts`, `POST /scripts/start`, `/stop`, `/pause`, `/resume`, `GET /scripts/status`, `/health`, `/results` |
| Estado de state machines | `GET /debug/snapshot?script=<nome>` |
| Configuração | `GET/POST /settings/plugin` (grupo, chave, valor) |
| Imagem | `GET /screenshot` |

## Arquitetura

O cliente do jogo roda no `zaros`; o planner roda no `zaros` agora (doc 04, seção 8) e no
`saradomin` depois. Para os dois casos funcionarem igual, o controle passa sempre por um **agent** no
`zaros`, que é o mesmo binário (`chronicle agent`) rodando como serviço systemd do usuário, declarado no
`nix-config` ao lado do módulo do Microbot.

```
navegador ──Zitadel (OIDC) + papel──> chronicle serve (saradomin; ou zaros no modo local-first)
                                            ▲
                                            │ websocket de SAÍDA, dentro do Tailscale,
                                            │ machine user do Zitadel; carrega comandos, eventos e o VNC
                                            │
                                 chronicle agent (zaros, serviço systemd do usuário)
                                            ├── UDS + token ──> Agent Server (cliente Microbot no Xvfb)
                                            ├── TCP 127.0.0.1:5999 ──> x11vnc ──> Xvfb :99
                                            └── exec ──> mb-client start|stop --headless
```

- **Só conexão de saída**: o agent conecta no `serve`; o `zaros` não abre porta nenhuma. A mesma
  conexão multiplexa comandos, eventos (estado do cliente e dos scripts) e o fluxo do VNC. No modo
  local-first o agent conecta em `localhost`, sem mudar código.
- **`internal/microbot`**: client Go tipado do Agent Server (HTTP sobre o socket Unix, token lido do
  arquivo a cada requisição, porque muda quando o cliente reinicia). Usado só pelo agent.
- **`internal/agent`**: o executor do lado do `zaros`. Recebe comandos tipados, valida contra a
  lista fechada (ver Segurança), executa e devolve resultado e eventos.
- **Ciclo de vida do cliente**: o agent não reimplementa o launch. Ele chama o `mb-client` do
  harness (`microbot/bin` do checkout do chronicle em `MICROBOT_HARNESS_DIR`), que já cuida de JDK, hash do jar, HWID agent,
  `user.home` isolado, Xvfb e VNC. Uma fonte de verdade para como o jogo sobe.
- **VNC no navegador**: o navegador abre `/api/microbot/vnc` no `serve`, que repassa os bytes pela
  conexão do agent até `127.0.0.1:5999`; no lugar do `websockify`. A SPA usa o noVNC
  (`@novnc/novnc`).
- **Toggle "só dashboard"**: com ele ligado, a tela não abre o websocket do VNC; mostra estado
  (`/state`), estado da state machine do plugin ativo, screenshot a cada poucos segundos e a posição
  num mapa. A preferência fica na sessão do usuário.
- **Atualização ao vivo**: SSE (já previsto no doc 04) com o estado do cliente e dos scripts, em vez
  de polling na SPA.
- **Rotinas**: ficam no Postgres e rodam dentro do `serve` (scheduler in-process do doc 04). Um único
  executor por cliente, com lock, porque só existe um personagem por vez.

### Modelo de rotina (rascunho)

```
routine        id, name, enabled, schedule (cron, opcional), created_at
routine_step   routine_id, position, kind, params (jsonb)
routine_run    id, routine_id, started_at, finished_at, status, trigger (manual|cron)
run_event      run_id, at, step_position, kind, detail (jsonb)   -- trilha de auditoria e evidências
```

Tipos de step: `client_start`, `login` (mundo), `plugin_install` (nome, versão), `plugin_config`
(grupo, chave, valor), `plugin_start` (nome), `wait_until` (plugin parou | resultado com campo X |
estado do jogo | timeout), `plugin_stop`, `logout`, `client_stop`. Uma falha para a execução, tira
screenshot e registra o motivo; nada é dado como concluído sem um estado observado (mesmo princípio
da regra 2 do CLAUDE.md).

## Segurança

Quem controla este módulo controla a conta do jogo. Tailscale resolve a rede e o Zitadel resolve
quem é o usuário; as camadas abaixo cobrem o resto.

1. **Desligado por padrão** (`MICROBOT_CONTROL=off`); rotas `/api/microbot/*` só existem quando ligado.
2. **Autorização, não só autenticação**: toda rota de controle exige o papel `microbot:control`
   atribuído no Zitadel (claim de roles). Em `AUTH_MODE=dev` o servidor só escuta em loopback ou na
   interface do Tailscale.
3. **Credencial de máquina para o agent** (decidido com o dono em 2026-11-08): em `AUTH_MODE=dev` o
   agent entra sem token e só do loopback, como as rotas de controle. Em `oidc` ele é um machine user
   do Zitadel com o papel `microbot:agent` (que nenhuma pessoa tem): pega o access token pelo
   `client_credentials`, com os escopos que `GET /api/auth/client` dá em `agent_scopes` (papéis do
   projeto e a audiência `OIDC_PROJECT_ID`), e o `serve` o valida offline pelo JWKS
   (`Provider.VerifyMachineToken`). O segredo do machine user fica com sops no `nix-config`
   (`MICROBOT_AGENT_CLIENT_SECRET_FILE`); revogar é no Zitadel. Reforço: o `serve` confere pela
   identidade do Tailscale (`WhoIs`) que a conexão vem do `zaros`.
4. **Lista fechada de comandos no agent** (a camada mais importante): o agent nunca repassa
   requisições cruas ao Agent Server. Só executa comandos tipados: abrir e fechar o cliente,
   login/logout, instalar/remover plugin do Hub, iniciar/parar/pausar script, ler estado, ler e gravar
   configuração, screenshot e a sessão de VNC. Ficam **fora**: `/scripts/deploy` (compila e executa
   Java arbitrário), teclado e widgets crus, e qualquer exec além do `mb-client`. Assim, mesmo um
   `saradomin` comprometido não consegue executar código no `zaros`.
5. **ACL do Tailscale**: o `zaros` alcança só o `saradomin` na porta do planner; nada na tailnet
   inicia conexão para o `zaros`.
6. **Controle local**: `systemctl --user stop chronicle-agent` (ou `chronicle agent pause`) corta o
   controle do planner no `zaros`. O jogo continua aberto: o serviço para só o agent
   (`KillMode=process`), e o cliente fecha pelo Cockpit ou com `mb-client stop`. O agent não inicia ações enquanto o Microbot indicar input humano (o dono jogando).
7. **Web**: sessão exigida também no websocket do VNC, checagem de `Origin`, proteção contra CSRF
   nas rotas que alteram estado.
8. **Segredos fora do navegador**: o token do Agent Server e o VNC nunca saem do `zaros`; o
   navegador só fala com o `serve`.
9. **Auditoria**: toda ação de controle e toda execução de rotina (`run_event`), com usuário, origem
   e resultado.

## Lacunas conhecidas

- **Esquema de configuração dos plugins** (resolvido em 2026-11-08, hoje pelo plugin Chronicle Link 1.4.0, ver
  "Configuração com formulário"): o Agent Server só lê e grava chave/valor; não lista as
  opções de um plugin (nome, tipo, valores, seções). V1: edição chave/valor crua mais presets salvos
  por plugin. V2: endpoint de descritores de config (`ConfigDescriptor`), de preferência como PR
  upstream no Agent Server, ou num plugin companheiro em `microbot/`.
- **GPU no modo headless**: desligada. Uma saída headless do Hyprland com `wayvnc` daria GPU real,
  mas depende da sessão gráfica aberta; fica como modo opcional futuro.

## Fases

1. `internal/microbot` (client do Agent Server) e `chronicle agent` com a lista fechada de comandos,
   conectando no `serve` local; status do cliente, iniciar/parar via `mb-client`, login/logout,
   screenshot; tela **Microbot** com o toggle e o modo só dashboard; papel e auditoria.
2. VNC ao vivo: fluxo pela conexão do agent e noVNC na SPA.
3. Plugins: listar, instalar do Hub, iniciar/parar, status/saúde/resultados, snapshot da state
   machine, config chave/valor.
4. Rotinas: modelo, executor com lock, agendamento, histórico com screenshots.
5. Agent remoto: serviço systemd no `nix-config`, machine user do Zitadel com o segredo via sops, `WhoIs` do Tailscale e
   ACL; junto com o deploy do planner no `saradomin`. Como o agent existe desde a fase 1, aqui muda
   só configuração. **Serviço pronto, desligado** (2026-11-13, PR #1 do `nix-config`, com
   `docs/chronicle-agent.md` de lá): o aspecto `chronicle-agent` roda o `chronicle agent` como
   serviço de usuário com linger, a partir do checkout em `harnessDir`, com o cliente sempre
   headless; o segredo do machine user vem do sops em `/run/secrets` e chega só como
   `MICROBOT_AGENT_CLIENT_SECRET_FILE`. Para ligar, o dono cria o machine user com o papel
   `microbot:agent`, põe o segredo em `secrets/chronicle-agent.yaml`, importa o aspecto no zaros
   e deixa de rodar o `just agent` à mão (os passos estão no PR). O agent roda o branch que estiver
   no checkout: apontar `harnessDir` para uma worktree só do `main` evita surpresa.

## Fase 4: rotinas com atividade principal e runs com relógio (desenho, 2026-11-09)

**Estado: proposta para o dono revisar antes de codar.** Substitui o rascunho de "Modelo de rotina"
acima no que se sobrepõe e segue o doc 10 (decisão 31 do doc 01: recorrentes com `ready_at`;
`train` por delegação a plugins de terceiros).

### O cenário

Pedido do dono (2026-11-09): fazer uma atividade longa (o fishing de hoje, com o Tick Fisher) e, ao
mesmo tempo, manter herb runs, tree runs, birdhouse runs e o gato (cuidar e trocar). Três tipos de
coisa, com regras diferentes:

| Tipo | Exemplo | Como roda |
|---|---|---|
| Principal | Tick Fisher (barbarian fishing) | fica ligado enquanto nada mais precisa do personagem |
| Recorrente com relógio | herb run (Herbrun), birdhouses (FornBirdhouseRuns), árvores (FarmTreeRun) | quando vence, interrompe o principal, roda e devolve |
| Paralela | gato (KittenPlugin: fome, atenção, crescimento) | fica ligada junto do principal; ações curtas |

Fato observado: na sessão do dono o `KITTENSCRIPT` já rodava ao lado do `TickFisherPlugin`, então o
paralelo funciona hoje. O que falta é o meio: parar o principal num ponto seguro, rodar o run e
voltar.

### O relógio

Ordem das fontes, da melhor para a pior:

1. **Time Tracking do RuneLite pela ponte**: `FarmingTracker.getSummary(Tab)` e
   `getCompletionTime(Tab)` (HERB, TREE, FRUIT_TREE, ALLOTMENT, …) e `BirdHouseTracker`, os mesmos
   números do painel Time Tracking do cliente. Os varbits de canteiro só valem perto do canteiro
   (são compartilhados entre regiões), por isso não dá para ler do ingest; o Time Tracking registra
   quando o personagem passa pelo canteiro e prevê pelos ciclos de crescimento. A ponte ganharia
   `GET /bridge/timers`; o spike é alcançar a instância do plugin (pelo `PluginManager`) ou, se não
   der, ler as chaves persistidas do grupo `timetracking` do perfil e recalcular.
2. **Intervalo fixo desde o fim do último run** (herb 80 min, birdhouse 50 min, árvore pelo tipo),
   quando o Time Tracking ainda não conhece o canteiro.
3. **Evento de chat**, para o que não tem relógio: "Your kitten has grown into a healthy cat" dispara
   a troca do gato.

O `ready_at` de cada recorrente vira fato no planner (decisão 31): aparece no Agora e na Automação
mesmo sem rotina ligada, o que já tem valor sozinho.

### A troca (onde está o difícil)

Estados do executor, um por cliente, com lock (só existe um personagem):

```
PRINCIPAL ──(recorrente vence e não há break)──▶ PARANDO ──▶ PREPARO ──▶ RUN ──▶ RETORNO ──▶ PRINCIPAL
     ▲                                                                     │
     └──────────────── falha ou timeout: para tudo, screenshot, avisa ◀────┘
```

- **PARANDO**: guarda a posição e para o plugin principal (`script_stop`, não `scripts_pause`, que
  pausaria o gato também); espera o personagem ficar parado (sem animação nem movimento por ~3 s).
- **PREPARO**: o run precisa do inventário certo (sementes, compost, spade, rake, teleports). Duas
  saídas: o plugin do Hub faz o próprio banco (cada um precisa ser verificado), ou a rotina declara um
  loadout e o executor usa o banco antes (o planner já sabe ler posse e loadout, `internal/core/planning`).
- **RUN**: liga o plugin do run com o preset de config salvo. Fim por, nesta ordem: o plugin se
  desligar sozinho (`/scripts/status`), a state machine chegar ao estado final (`/debug/snapshot`), ou
  timeout por tipo (herb run: 15 min). Sucesso só com estado observado (regra 2 do CLAUDE.md): o Time
  Tracking mostra os canteiros plantados de novo, ou o chat confirma.
- **RETORNO**: `walk` (webwalker) até a posição guardada e liga o principal com o mesmo preset.
- **Paralelos** continuam ligados no PRINCIPAL; durante PREPARO e RUN ficam desligados, para um afago
  no gato não roubar o mouse no meio de um teleporte.
- **Breaks**: com o Break Handler ligado, nada começa durante um break. Falta verificar como ele
  trata um run em andamento (se espera o script ou desloga no meio); até lá, o executor não começa
  um run quando o próximo break está a menos que o timeout do run. Desde 2026-11-13 (Chronicle Link
  1.12.0) o break já é observável: a ponte responde `GET /bridge/break` e o `status` do agent o leva
  em `break` (`active`, `in_s` até o próximo, `left_s` do atual; doc 06, adendo de 2026-11-13). O
  vigia das recorrentes usa isso (2026-11-13): uma rotina que vence espera, com o motivo no
  `recurring_get`, se há um break em andamento ou se o próximo vem antes do timeout do run (15 min
  quando o run não tem timeout próprio, nunca mais de 30 min, para um timeout longo não segurar a
  rotina em todo intervalo entre breaks). Sem a ponte 1.12, ou com o Break Handler V2, que não diz
  quando, nada espera.
- **Dono no controle**: abrir o VNC em modo intervir pausa o executor; ele não retoma sozinho.

### Modelo de dados (amplia o rascunho)

```
routine          id, client, name, enabled, created_at
routine_task     routine_id, kind (principal | recorrente | paralela), plugin (className),
                 preset (config jsonb), trigger (timetracking:<tab> | interval:<min> | chat:<texto>),
                 loadout (jsonb, opcional), return_to (posição | nenhuma), timeout_s, priority
routine_run      id, routine_id, task, started_at, finished_at, status, trigger
run_event        run_id, at, state, kind, detail (jsonb)    -- trilha, screenshots e evidências
```

### Entregas

1. **Relógios** (só leitura): `/bridge/timers` na ponte e um cartão "Relógios" no cockpit (herb em
   23 min, birdhouses prontas, árvores amanhã 14:10), mais o `ready_at` como fato. Sem risco e útil
   já.
2. **Fazer agora**: o executor com uma recorrente e o principal, disparado pelo dono num botão
   ("trocar o gato agora e voltar a pescar"). Valida PARANDO, RUN, fim e RETORNO com o dono olhando.
   Reordenado em 2026-11-09: a conta ainda não tem os requisitos dos runs com relógio (Farming 1,
   sem Bone Voyage), então a primeira recorrente é a troca do gato.
3. **Evento**: a troca do gato disparada pela mensagem "Your kitten has grown into a healthy cat that
   can hunt for itself." (o KittenPlugin já a vê). Feita em 2026-11-10 pelo Chronicle Link (a ponte serve o evento) e por
   um gatilho no agent (ver Estado).
4. **Automático e preparo**: o relógio dispara herb, tree e birdhouse quando a conta tiver os
   requisitos, várias recorrentes em fila por prioridade, e loadout com banco antes do run.
   O relógio, o intervalo e a fila por prioridade foram feitos em 2026-11-13 (ver Estado); o
   loadout com banco ficou de fora.

O ciclo do gato (resposta do dono, R3): o filhote fica com o KittenPlugin (paralela: alimentar e
afagar até crescer); quando cresce, a troca é uma recorrente disparada pelo evento, que leva o gato
a um Civilian em West Ardougne e troca por death runes, compra um filhote novo com a Gertrude e
volta ao principal. Precisa de coins no inventário para o filhote. Sem plugin conhecido no Hub para
isso, é candidato a plugin nosso em `microbot/`.

A tela "Rotinas" do canvas (V6MicrobotRotinas1440, página 5) foi desenhada para passos em sequência;
este cenário pede outra forma (o principal, a fila de recorrentes com o relógio de cada uma e a
trilha da última troca). Desenhar no canvas antes da entrega 2.

### Decisões para o dono

| # | Pergunta | Estado |
|---|---|---|
| R1 | O executor roda no `serve` (estado e auditoria num lugar só) ou no agent, ao lado do cliente (sobrevive a queda do link durante um run)? | decidida (2026-11-09): no agent. O dono considera o cliente headless num mini PC com Intel N95, sempre ligado. Recomendação revista: **no agent**, porque o `serve` reinicia (o `air` a cada mudança em Go no `just dev`, deploys no saradomin) e o link passa a ser remoto; o `serve` guarda as rotinas, dispara o "fazer agora" e recebe a trilha, e o agent manda as ações para a auditoria (com fila se o link cair). Só pesa a partir da entrega 2 |
| R2 | Delegar aos plugins do Hub (Herbrun, FornBirdhouseRuns, FarmTreeRun) ou escrever os nossos (hoje em `microbot/`)? | decidida (2026-11-09): os dois. A tarefa aponta para uma classe de plugin com um preset, venha do Hub ou de `microbot/`; começar pelos do Hub e escrever o que faltar (fim sem aviso, sem banco, a troca do gato) |
| R3 | "Trocar de gato" é o quê? | decidida (2026-11-09): o ciclo do gato descrito nas entregas |
| R4 | Começar pela entrega 1 (relógios) agora? | decidida (2026-11-09): sim, feita (ver Estado) |

### Cliente headless num N95

Medido no N95 em 2026-11-12, e a proposta de workers (um pod por conta no saradomin): doc 11.

Plano do dono (2026-11-09): o webapp roda no saradomin, como já previsto; o agent e o cliente
headless vão para o N95 e, se faltar CPU, para o zaros. Com o executor no agent (R1), ele vai junto
com o cliente e trocar de máquina é só onde o `chronicle agent` roda, sem mudança de código.

Medido no zaros (Ryzen 7 7800X3D) em 2026-11-09, com o cliente de dev **em janela** pescando (Tick
Fisher e Kitten ligados, o cockpit lendo): 1,1 a 2 núcleos e 2,7 GB de RSS (`-Xmx2g`), com o 117 HD
desenhando na RTX 4070 SUPER e FPS sem limite (`fpscontrol.limitFps=false`). Esse número não serve
para o headless: lá não há GPU (Xvfb), o 117 HD fica desligado e o cliente volta ao renderizador do
jogo, que desenha no CPU. O que muda o custo é o FPS: limitar no FPS Control (15 a 20; o bot age por
tick de 600 ms). O N95 tem 4 núcleos e 4 threads, com cerca de metade do desempenho por núcleo do
7800X3D; um cliente deve caber, dois provavelmente não. Sem o 117 HD a memória também cai (no início
ele deixou 66 MiB livres dos 2 GiB); 8 GB de RAM bastam para um. O cockpit pesa no cliente: o
`/observe` com o que está por perto custa ~2 s, então ler menos quando ninguém olha.

Antes de decidir: medir o headless com FPS limitado (CPU por 10 min fazendo a atividade principal)
no zaros, e depois no N95. Levar o cliente para outra máquina leva junto o harness (Nix), a sessão
da conta (`mb-account`) e os IDs de
aparelho do HWID agent (`~/.runelite/spoofed-*.properties`); quem move isso é o dono.

## Estado (2026-11-08)

**Fase 1 feita.** O que existe:

- `internal/microbot`: cliente do Agent Server pelo socket Unix do home de dev do harness, com o token
  relido a cada chamada. O Agent Server confere o `Host`: o cliente manda `127.0.0.1`, como o `mb_api`
  do harness.
- `internal/agent`: protocolo, executor da lista fechada (`status`, `client_start`, `client_stop`,
  `login`, `logout`, `screenshot`) e o link. O agent abre um websocket de **saída** para
  `/api/microbot/agent` (autenticado conforme o modo, item 3 de Segurança), se apresenta e responde
  comandos por id; reconecta sozinho (1 s a 30 s). O lado do `serve` aceita um agent por vez (uma
  conexão nova substitui a antiga) e recusa comando fora da lista nos dois lados.
- `client_start` roda `mb-client start --headless` pelo dev shell Nix do harness
  (`nix develop $MICROBOT_HARNESS_DIR#microbot --command $MICROBOT_HARNESS_DIR/microbot/bin/mb-client`, ou
  `MICROBOT_CLIENT_CMD`; um checkout antigo do `microbot-plugins`, sem `microbot/bin`, ainda roda pelo
  shell padrão dele e `bin/`);
  `client_stop` roda `mb-client stop`. O screenshot sai do Agent Server em PNG (megabytes no cliente em
  janela) e o agent o recodifica em JPEG antes do link.
- `serve` com `MICROBOT_CONTROL=on` (exige `DATABASE_URL`; em oidc, `OIDC_PROJECT_ID`):
  `GET /api/microbot/status`, `GET /api/microbot/screenshot`, `GET /api/microbot/audit`,
  `POST /api/microbot/client/start|client/stop|login|logout`. Em dev, só de um peer loopback; em oidc,
  a sessão ou o bearer e o papel `microbot:control`. Toda ação (não as leituras) vai para
  `microbot_audit` (migration 0010) com quem, origem e resultado, inclusive as que falham.
- Web: Automação › Cockpit (`/automacao`, artboards V6Microbot1440 e V6Microbot640): estado do cliente,
  da conta e do agent, a tela por screenshot a cada 5 s, o `/state` a cada 3 s, iniciar e parar o
  cliente, login (com mundo opcional) e logout com segundo clique de confirmação, e a auditoria. A
  navegação só mostra "Automação" quando o servidor tem o módulo ligado.
- Rodar no zaros: `MICROBOT_CONTROL=on` no `.env` do planner e `just agent` (em dev sem token;
  `MICROBOT_HARNESS_DIR` é o próprio checkout do chronicle por padrão). Contra um `serve` em oidc:
  `MICROBOT_AGENT_CLIENT_ID` e `MICROBOT_AGENT_CLIENT_SECRET` (ou `_SECRET_FILE`) do machine user.
- Verificado no zaros: o agent leu o cliente em janela que estava aberto (estado, mundo e screenshot),
  subiu o headless pelo web em 38 s (tela de login, deslogado), tirou screenshot dele e o parou; a
  auditoria registrou as duas ações. Login e logout ficaram nos testes com Agent Server falso, porque
  logam a conta.

**Fase 2 feita** (VNC ao vivo):

- Comando `vnc_open` na lista fechada. O navegador abre o websocket `/api/microbot/vnc` (mesmas
  checagens das outras rotas de controle; a biblioteca de websocket confere `Origin` contra `Host`, então
  outro site não abre o VNC com o cookie do dono). O `serve` pede ao agent uma sessão com um id; o agent
  conecta no `x11vnc` em `127.0.0.1:$MB_VNC_PORT` (padrão 5999) e abre um segundo websocket de **saída**,
  `/api/microbot/agent/vnc?session=<id>`, autenticado como o link; o `serve` emenda os dois fluxos de bytes
  RFB. A máquina do jogo continua sem porta aberta e o VNC nunca sai do loopback. Cada sessão aberta
  (ou que falhou ao abrir) vai para a auditoria.
- Web: o seletor "só dashboard / VNC ao vivo" do V6Microbot1440 e o noVNC (`@novnc/novnc` 1.7.0,
  carregado só ao abrir). A sessão abre só olhando (`viewOnly`); "Intervir com mouse e teclado" passa
  o input ao navegador, "Devolver: só olhar" o tira. No celular (abaixo de 700 px) não há VNC, como no
  V6Microbot640. O screenshot periódico para enquanto o VNC está aberto.
- O `viewOnly` é do navegador: quem abre o VNC pode mandar input. O controle de quem abre é o mesmo das
  outras ações (dev só loopback, oidc com o papel), e a abertura fica na auditoria.
- Verificado no zaros: o cliente headless subiu pelo web e a tela de login apareceu ao vivo no cockpit
  pelo túnel; a sessão fecha quando a página sai.

**Fase 3 feita** (plugins):

- Comandos tipados na lista fechada: `plugins` (`GET /scripts` e `GET /plugins?installed=true`),
  `hub_search`, `plugin_install` e `plugin_remove` (pelo `internalName` do Hub), `script_start` e
  `script_stop` (pelo nome completo da classe), `scripts_pause` e `scripts_resume` (todos os loops, como
  o Agent Server faz), `script_detail` (`/scripts/status`, `/health`, `/results` e `/debug/snapshot`),
  `config_get` e `config_set` (`/settings/plugin`). A validação fica em `agent.ValidateParams`: o `serve`
  a usa para recusar com 400 antes de falar com o agent (e sem auditar, porque nada chegou a ele), e o
  agent a usa de novo antes de agir.
- Recusas da lista fechada: iniciar ou parar o núcleo do controle (Microbot, Agent Server, Dev
  Bootstrap) e o plugin de tracking do planner; ler ou gravar os grupos `agentServer` (porta, token),
  `runelite`, `microbot` e `fm-ironmanplanner` (credencial do planner); chaves que parecem segredo
  (token, senha, cookie, PIN); nomes fora do formato (só o `internalName` do Hub, classe Java
  qualificada, grupo e chave `[A-Za-z0-9_.-]`).
- Web: Automação › Plugins (`/automacao/plugins`, V6MicrobotPlugins1440). A lista junta o `/scripts`
  com o Hub pelo nome simples da classe (é o `internalName`), em três grupos: do Hub, seus (pacote `fm`,
  sideload) e do cliente (escondidos por padrão). O detalhe tem iniciar e parar (segundo clique),
  pausar e retomar todos, atualizar e remover (Hub), status, saúde e último resultado, os JSONs da state
  machine e dos resultados, e a configuração chave/valor: o grupo (palpite pelo nome da classe) e as
  chaves de cada plugin ficam neste navegador, porque o Agent Server não lista as opções. A busca no
  Hub instala ou atualiza.
- Verificado no zaros com o cliente headless na tela de login, só leituras: a lista (26 plugins, 5 do
  Hub, 2 com atualização), a busca no Hub, o detalhe, a state machine e a leitura de uma chave; o grupo
  `agentServer` foi recusado. Instalar, iniciar, parar e gravar config ficaram nos testes com Agent
  Server falso.

**Ver e agir no jogo** (2026-11-08, pedido do dono depois da fase 3):

- `game_view` (leitura, sem trava): `GET /observe` com o que está por perto (estado, inventário,
  equipamento, diálogo, chat do jogo, nunca de jogadores, erros e heartbeats), mais `/inventory` (as
  ações de cada item), `/skills`, `/bank`, `/ground-items` e `/debug/snapshot`. Deslogado, só o
  `/observe`. Rota `GET /api/microbot/game`.
- Ações tipadas na lista fechada, em `POST /api/microbot/game/<ação>`, validadas dos dois lados e
  auditadas: `walk` (webwalker até um tile, sem bloquear), `inventory_use`, `inventory_drop`,
  `bank_open`, `bank_close`, `bank_deposit`, `bank_withdraw`, `npc_interact`, `object_interact`,
  `ground_pickup`, `dialogue_continue` e `dialogue_select`. Nome ou id (nunca os dois), ação como o
  menu do jogo mostra, uma linha imprimível; coordenadas no intervalo do mundo. Fora: teclado e
  widgets crus e o Grand Exchange (conta ironman).
- Web, no Cockpit com a conta logada: inventário em grade e equipamento no boneco (células e sprites
  da Conta), o painel Ação (clicar num item, NPC, objeto ou item do chão mostra o que dá para fazer:
  as ações do item, dropar e depositar com segundo clique, andar até, ações comuns do NPC ou objeto
  e um campo livre, pegar do chão, sacar do banco), andar até coordenadas, diálogo com as opções e
  continuar, por perto, script ativo (state machines e heartbeats), banco (abrir o mais perto,
  depositar tudo, fechar), skills e o chat do jogo, lidos a cada 3 s. Com scripts de loop com
  heartbeat recente e não pausados, a tela avisa e oferece "Pausar todos os scripts", sem travar:
  plugins ligados mantêm loops passivos o tempo todo (PIN do banco, ataque especial, antiban), e
  travar deixaria as ações sempre desligadas.
- A distância dos NPCs sai da posição (tiles, como o RuneLite): o `distance` do Agent Server vem em
  unidades locais (128 por tile).
- Verificado com dados falsos no navegador em 1440 e 640, e com a conta logada pelo dono, só lendo:
  inventário, equipamento, por perto, script ativo e skills de verdade.

**Logs e conta** (2026-11-08):

- `client_logs` (leitura): o fim do `client.log` do home de dev (ou do `run/client.out`), como
  `mb-client logs`, com o que parece segredo mascarado no agent (pares token/sessão/senha, JWT,
  sequências opacas de 32+ caracteres). `GET /api/microbot/client/logs?lines=`.
- `account_show` (leitura) e `account_use` (pelo número na lista, 1 a 50): o agent roda
  `mb-account show`, `list` e `use <n>` pelo dev shell do harness (`MICROBOT_ACCOUNT_CMD` troca o
  comando); saem só os nomes, as sessões do launcher ficam no zaros. Trocar é recusado com o cliente
  rodando (vale no próximo início). `GET` e `POST /api/microbot/account`.
- `world_hop` (mundo 301 a 999): logout e login no mundo novo. `POST /api/microbot/world`.
- Web: "Trocar de mundo" na faixa do Cockpit (segundo clique), o painel "Conta do cliente" na coluna
  da auditoria (personagem atual e a lista, "Usar" com segundo clique e o cliente parado) e o "Log do
  cliente" (fechado por padrão; reler, acompanhar a cada 5 s, filtro).
- Verificado no zaros, só leituras: a lista de personagens e 300 linhas do log, com as máscaras só em
  hashes do Nix e dos jars.

**Configuração com formulário** (2026-11-08): fecha a lacuna "esquema de configuração dos plugins"
abaixo, pelo caminho do plugin companheiro.

- `microbot-plugins`, plugin Chronicle Link 1.4.0 (antes Ironman Planner 1.3) no cliente de dev (até 2026-11-09 era o Dev
  Bootstrap 1.1): escreve `.runelite/agent/config-schema.json` com as
  opções de cada plugin do Microbot carregado (grupo, seções, itens com tipo, intervalo, unidade,
  nomes e rótulos de enum e o aviso do plugin), sem os itens `secret`, e reescreve quando um plugin
  carrega, descarrega ou muda. Os valores não vão no arquivo: continuam no `/settings/plugin`.
- `config_schema` (leitura): o agent lê o arquivo e devolve o formulário de um plugin, com as regras
  do `config_get` (sem grupo protegido, sem chave que pareça segredo). `GET
  /api/microbot/config/schema?className=`.
- Web, em Plugins: o formulário por seção (as fechadas por padrão começam fechadas), com o controle
  do tipo (caixa para booleano, número com mínimo, máximo e unidade, lista para enum, texto para o
  resto, amostra de cor), "Gravar" por opção alterada (segundo clique quando o plugin tem aviso) e
  "padrão" para apagar a chave. Sem o arquivo (plugin antigo), a tela cai no chave/valor cru,
  que também fica a um clique como alternativa.
- Para ligar no zaros: `mb-install chroniclelink` (home de dev, nunca `--live`) e reiniciar o cliente.
- Verificado: testes do export (tipos e enums) no `microbot-plugins`, do agent (arquivo, grupo
  protegido, chave de segredo) e do formulário com dados falsos no navegador (validação, edição
  que sobrevive ao polling).

**Cockpit como o cliente do jogo** (2026-11-09; página 6 do canvas, aprovada pelo dono): a réplica
do cliente fixo, com a pele do jogo dentro e o Pergaminho fora.

- A tela (screenshot ou VNC) com o chat embaixo (mensagens do jogo e o diálogo com as opções
  clicáveis e o "Click here to continue") e, à direita, o painel lateral com as abas do jogo: Itens,
  Equipado, Skills, Oração e Magia. Embaixo, Por perto, Script ativo e Banco.
- Um só menu "Choose Option" (cores do jogo) para inventário, banco, chão, NPC e objeto; clique
  esquerdo ou direito abre e nada acontece sem escolher. Só aparecem as opções reais: as ações do
  item que o `/inventory` traz; com o banco aberto, Deposit-1, 5, 10 e All (o Microbot procura a ação
  no widget do inventário do banco, então passam pelo `inventory_use`); no banco, Withdraw-1, 5, 10 e
  All (o `bank_withdraw` ganhou `all`); no chão, Take e Walk here. Drop e Deposit-All pedem o segundo
  clique no próprio menu.
- Entrega 1 (feita): o que o agent já faz. Oração e Magia aparecem desabilitadas; NPC e objeto têm
  Walk here e a ação digitada, e o equipamento só mostra.
- Entrega 2 (a ponte, feita): o plugin Chronicle Link 1.4.0 do microbot-plugins (Ironman Planner até a 1.3; no começo era o
  Dev Bootstrap 1.2; em 2026-11-09 o dono preferiu um só plugin do Chronicle, doc 06) serve, só no
  cliente de dev do harness, um segundo socket,
  `.runelite/agent/bridge.sock`, com o `UdsHttpServer` e o `AgentHandler` do próprio cliente: o mesmo
  token do Agent Server, Host de loopback, sem Origin, e nada responde antes de o token existir.
  Leituras: `/bridge/entities` (NPCs e objetos por perto com as opções reais do menu),
  `/bridge/equipment` (itens vestidos com as ações), `/bridge/prayers` (cada oração com nível,
  liberada, ativa e os pontos) e `/bridge/spells` (o livro atual com nível e se dá para lançar).
  Ações: `/bridge/equipment/interact`, `/bridge/prayers/toggle`, `/bridge/spells/cast` (alquimia num
  item) e `/bridge/bank/deposit-x`.
- No planner, o agent fala com a ponte por um segundo `microbot.Client` (`Socket: bridge.sock`): o
  `game_view` traz a parte `bridge` (ou `errors.bridge` dizendo por que faltou) e a lista fechada
  ganhou `equipment_use`, `prayer_toggle`, `spell_cast` e `bank_deposit_x`, validados como as outras
  (chaves do cliente em `[A-Z0-9_]`, nome e ação numa linha, quantidade positiva).
- Web: com a ponte, Por perto mostra as opções reais de NPC e objeto (sem o campo digitado), os itens
  vestidos abrem o menu deles, Oração (halo nas ativas, apagadas as fora do nível, pontos) e Magia
  (apagados os sem nível ou runas) ligam, o banco ganha Deposit-X e Withdraw-X (o número no próprio
  menu; Withdraw-X já funcionava pelo `bank_withdraw`) e um feitiço de item (alquimia) seleciona e o
  próximo clique no inventário vira "Cast … -> item", como no jogo. Sprites de oração e feitiço vêm
  da wiki pelo `tools/fetch_wiki_icons.py` (lista em `data/wiki/book_icons.json`, as chaves do cliente).
- Para ligar no zaros: `mb-install chroniclelink` (home de dev) e reiniciar o cliente.

**Usar em, feitiços com alvo e mapa do mundo** (2026-11-09, pedido do dono depois de testar o
cockpit: "não sei se consigo usar uma magia em algum item do inventário", e um mapa para escolher o
destino da viagem):

- Seleção como no jogo: "Use" num item (o jogo põe Use em todo item, antes do Drop; as ações do item
  não o trazem, então a web o insere quando há ponte) ou Cast num feitiço com alvo seleciona; o
  texto "Use Knife ->" aparece no topo da tela com o item contornado, e o próximo clique num item,
  NPC ou objeto abre "Use Knife -> Teak logs" ou "Cast Wind Strike -> Goblin". O Cancel do menu, o
  link no topo e o Esc desfazem. A seleção vive só na web; o jogo recebe a ação inteira.
- Ponte (Chronicle Link 1.5.0): `POST /bridge/inventory/use-on` (item em outro item, em NPC pelo
  índice ou em objeto pelo id); `/bridge/spells/cast` aceita `npc` (combate, curses, grasps,
  demonbanes, pelo `castOn`) além de `item` (alquimia e agora Superheat); `/bridge/spells` lista
  também os 36 feitiços de combate (`Rs2CombatSpells`) e diz em cada um o `target`: `item`, `npc`,
  `none` ou `unsupported` (alvo que a ponte ainda não alcança: enchant de joia, orbs, Telekinetic
  Grab, outro jogador, canteiro, cabeças), que a web mostra apagado e a ponte recusa, para o jogo
  não ficar com o feitiço selecionado. Item ausente ou NPC que sumiu responde 409. Desde a 1.4.2 Por
  perto lista só objetos com alguma ação (some o cenário como Bullrushes).
- Agent: `item_use_on` na lista fechada e `npc` no `spell_cast` (um alvo só, índice abaixo de
  65536). O `game_view` lê a ponte em paralelo com o Agent Server e, com ponte, pula o `observe`
  com vizinhos (2 s no cliente; a ponte já traz NPCs e objetos com os menus): a leitura caiu de uns
  6 s para uns 3 s. A web faz uma leitura por vez, porque leituras sobrepostas esgotavam as
  conexões do navegador.
- Mapa do mundo: o orb no canto do minimapa (ou "Mapa" ao lado de "andar até") abre um Leaflet com
  os tiles renderizados de `mejrs/layers_osrs` (a base do mapa da OSRS Wiki), carregados do GitHub
  pelo navegador, com o crédito no canto (decisão do dono: online, sem baixar para o repositório).
  O jogador é o ponto amarelo, o último destino o vermelho; planos 0 a 3; clicar abre "Walk here
  x, y" no mesmo menu, que anda pelo `walk` (webwalker do Microbot); dá para digitar coordenadas.
- Para ligar no zaros: `mb-install chroniclelink` e reiniciar o cliente, o `just agent` e o
  `just dev`.

**Fase 4, entrega 1: relógios** (2026-11-09, R4 aceita pelo dono):

- `microbot-plugins`, Chronicle Link 1.6.0: `GET /bridge/timers` lê os rastreadores do próprio
  plugin Time Tracking (`FarmingTracker`, `BirdHouseTracker`, `FarmingContractManager`, singletons
  do injector dele, pela API pública, sem reflexão): cada aba de farming, as birdhouses e o
  contrato, com o resumo (`COMPLETED`, `IN_PROGRESS`, `OCCUPIED`, `EMPTY`, `UNKNOWN`) e, crescendo
  ou pronto, quando fica pronto (segundos desde a época). Com o Time Tracking desligado,
  `enabled: false` e nada.
- Agent: `timers` (leitura) na lista fechada; `GET /api/microbot/timers`.
- Web: o cartão "Relógios" no topo da coluna da direita do Cockpit (artboard
  V6MicrobotRelogios1440, página 6c do canvas): pronto primeiro, depois o que cresce pela hora
  ("em 23 min", a hora no title), plantado e vazio; os que o Time Tracking nunca viu neste perfil
  numa linha "Não vistos" (desconhecido não é vazio). Relê a cada minuto e conta o tempo entre
  leituras pelo relógio da máquina do jogo.
- Verificado: testes do plugin, do agent e da view; o cartão no Cockpit com relógios falsos.
  Falta ver com o cliente de verdade (`mb-install chroniclelink` e reiniciar o cliente e o
  `just agent`).
- Ainda não: o `ready_at` como fato no planner (Agora), que entra com o executor (entrega 2).

**Fase 4, entrega 2: fazer agora, com a troca do gato** (2026-11-09):

- `microbot-plugins`, plugin **Cat Swap 1.0.0** (`fm/catswap`): uma troca e se desliga. Pega o gato
  crescido (adulto, lazy, wily, overgrown ou hell; filhote nunca), anda até os Civilians de West
  Ardougne (porta do muro de Ardougne, aberta pela Biohazard) e usa o gato num deles (é o que dá as
  runas; o Talk-to só conversa), anda até a Gertrude e compra com o "Kitten" do menu (100 coins, sem
  diálogo), solta o filhote para seguir. Recusa sem gato crescido, com o filhote ainda crescendo ou
  sem 100 coins no inventário. O resultado (`outcome` done ou failed, o motivo, as death runes
  ganhas) vai para o `ScriptResultStore` antes de o plugin se desligar.
- Agent: `run_start`, `run_status` e `run_cancel` na lista fechada. O executor roda numa goroutine
  do agent (decisão R1): guarda a posição, para os plugins marcados, espera o personagem ficar
  parado, liga o plugin do run, espera ele se desligar sozinho (timeout de 30 min por padrão), lê o
  último resultado (`outcome: failed` falha o run), volta ao lugar pelo webwalker (reemite o walk
  quando o personagem para no caminho) e religa o que parou. Falha, timeout ou cancelamento deixam
  o principal parado para o dono olhar. Enquanto um run dirige o personagem, o agent recusa as
  outras ações (só leituras e o cancelamento passam). `GET /api/microbot/run`, `POST
  /api/microbot/run` e `POST /api/microbot/run/cancel`; começar e cancelar vão para a auditoria.
- Web: o painel "Fazer agora" no Cockpit, abaixo dos Relógios (artboard V6MicrobotFazer1440,
  página 6d do canvas): o plugin a rodar (o Cat Swap primeiro), os plugins ligados a parar e religar
  (o Kitten fica ligado por padrão, porque precisa ver "Gertrude gives you another kitten." para
  zerar o crescimento), voltar ao lugar, e o botão com segundo clique. Rodando: o estado, a trilha e
  cancelar. Depois: o último run com o resultado (+100 death runes, filhote seguindo).
- Verificado: testes do Cat Swap (ids de seguidor e de item; filhote nunca conta como gato), do
  executor (a ordem parar, rodar, andar, religar; falha sem religar; ações recusadas durante o run;
  cancelar para o plugin) e da view; o painel no Cockpit com runs falsos. **Falta o teste no jogo**,
  com o dono logado e olhando: `mb-install catswap chroniclelink`, reiniciar o cliente e o
  `just agent`.
- Lacunas conhecidas: a trilha vive na memória do agent (some se ele reiniciar) e as ações que o
  executor faz sozinho não vão uma a uma para a auditoria (só o começar e o cancelar); cancelar
  durante a volta não para o webwalker já em curso.

**Fase 4, entrega 3: sozinho, a troca do gato quando o filhote cresce** (2026-11-10):

- `microbot-plugins`, **Chronicle Link 1.7.0**: a ponte ganha `GET /bridge/events?after=<seq>`, os
  eventos do jogo que os gatilhos esperam. O plugin olha o chat e registra só mensagens do próprio
  jogo de uma lista fechada (hoje "Your kitten has grown into a healthy cat", `kitten_grown`), nunca
  chat de jogador, para ninguém disparar um gatilho digitando o texto. Os eventos são numerados desde
  o início do plugin, com um `boot` que muda quando ele reinicia.
- Agent: `trigger_get` e `trigger_set` na lista fechada. O gatilho (o evento e o run que ele faz, os
  mesmos parâmetros do `run_start`) fica num arquivo da máquina do jogo
  (`~/.config/chronicle/agent-triggers.json`), então vale com o cockpit fechado, com o `serve` fora e
  depois de o agent reiniciar. Um vigia ao lado do link lê a ponte a cada 5 s e, com o gatilho ligado,
  começa o run salvo, marcado com o evento que o disparou. Eventos de antes de o vigia ver aquele
  início do plugin não disparam: um agent que reinicia depois de uma troca não troca de novo. Com o
  gatilho desligado ou com outro run em andamento, o evento só fica anotado (`last`).
  `GET /api/microbot/triggers` e `POST /api/microbot/triggers` (vai para a auditoria).
- Web: o bloco "Sozinho" no painel Fazer agora (artboard V6MicrobotSozinho1440, página 6e do canvas):
  "Quando o filhote crescer, fazer isto" salva as escolhas do painel como o run do gatilho, com
  segundo clique; ligado, mostra o que vai fazer e Desligar; a última vez que o filhote cresceu
  (disparou, gatilho desligado, outro run); "sem ler o chat" quando a ponte não serve os eventos. Um
  run disparado assim aparece como "sozinho".
- Verificado: testes do plugin (só mensagens do jogo contam; eventos depois de um seq), do agent
  (dispara o run salvo uma vez; evento antigo e início novo do plugin não disparam; gatilho desligado
  só anota; o arquivo vale para outro agent) e da view; o bloco no Cockpit com gatilhos falsos.
  **Falta o teste no jogo**: `mb-install chroniclelink`, reiniciar o cliente e o `just agent`, ligar
  o gatilho e esperar o filhote crescer, com o dono olhando a primeira vez.
- Lacunas conhecidas: além das da entrega 2, uma mensagem que aparece com o agent desligado se perde
  (o filhote cresce uma vez; aí é o "Fazer agora"). O lugar ruim para sair andando foi resolvido logo
  depois com o ponto seguro (abaixo).

**Fase 4: ponto seguro e histórico dos runs** (2026-11-10, pedido do dono depois da entrega 3):

- Ponto seguro: todo run, do dono ou do gatilho, começa no estado `waiting` e só para o principal
  quando o personagem está logado (fora de um break), sem barra de vida (fora de combate), fora de
  diálogo (`/dialogue` do Agent Server) e fora das regiões de minigame, raid e boss (a lista do plugin
  Discord do RuneLite, mais o Fishing Trawler afundando). Cada motivo novo vai para a trilha
  ("esperando: em Fishing Trawler"); sem ponto seguro em 60 min o run falha sem ter tocado em nada.
  Não pega instância fora dessas regiões: o Agent Server não diz se a região é instanciada.
- Histórico: o agent grava os últimos 30 runs com a trilha em `~/.config/chronicle/agent-runs.json` a
  cada linha da trilha. Depois de reiniciar, o último run volta a ser o atual (o "Último" do Fazer
  agora) e a numeração continua; um run que o agent não terminou aparece como falhou ("o agent parou
  no meio do run"). `run_history` (leitura) na lista fechada; `GET /api/microbot/runs`.
- Web: o painel "Runs" abaixo do Fazer agora (artboard V6MicrobotRuns1440, página 6f do canvas): o
  mais novo primeiro, "sozinho" nos do gatilho, resultado, quando, quanto levou, o que o plugin
  deixou, e a trilha ao clicar; o estado "esperando um ponto seguro" no Fazer agora.
- Verificado: testes do agent (espera diálogo, combate e Trawler na ordem e só depois para o
  principal; desiste sem tocar em nada; o arquivo vale para outro agent, ids continuam, run morto vira
  falhou, guarda só os últimos 30; tudo com `-race`) e da view; o painel no Cockpit com runs falsos.
- Desde 2026-11-13 (doc 11, F6, seção 16) os gatilhos e o histórico vão também para o banco do
  planner: o agent manda cada run pelo link (mensagens `event`) e recebe os gatilhos da conta a cada
  conexão (`routines_sync`). Os arquivos continuam como cache do agent anexado; um agent anterior à F6
  segue só com eles.

**Fase 4, entrega 4: rotinas recorrentes com relógio** (2026-11-13; modelo e API no doc 11, seção
16.4):

- **O que é:** um run (os parâmetros do `run_start`) com nome, prioridade (0 a 100) e o que o faz
  vencer:
  - o relógio do Time Tracking que ele segue: as chaves do `/bridge/timers` (`HERB`, `TREE`,
    `FRUIT_TREE`, `BIRD_HOUSE`, as outras abas de farming e `FARMING_CONTRACT`);
  - um intervalo de 10 min a 7 dias;
  - ou os dois.
- **Onde fica:** no Postgres (migração 0015), e vai ao agent no `routines_sync`. O agent anexado
  guarda uma cópia em `agent-recurring.json`.
- **Quando vence:**
  - Relógio conhecido: quando está pronto (`COMPLETED`, ou `IN_PROGRESS` com a hora já passada).
    Crescendo, ocupado ou vazio, não vence.
  - Relógio desconhecido ou sem relógio: pelo intervalo desde o fim do último run dela. É
    desconhecido quando o Time Tracking nunca viu os canteiros, está desligado ou a ponte não
    responde. Se a rotina nunca rodou, vence já.
- **O vigia** do agent olha a cada 30 s, e logo que chega um `routines_sync`:
  - não começa nada enquanto um run anda;
  - das vencidas, começa a de maior prioridade (no empate, pelo nome);
  - só começa com o personagem num ponto seguro (o mesmo do run), o que deixa de fora um break em
    curso (deslogado).
  - O run leva o id da rotina, e a primeira linha da trilha diz por que começou ("rotina Herb run:
    relógio Herb Patches pronto").
- **Travas** contra laço e surpresa:
  - um run que falhou ou foi cancelado segura a rotina até o dono salvá-la de novo;
  - um run de qualquer tipo que falhou ou foi cancelado segura todas até o dono salvar alguma (ele
    deixa o principal parado onde falhou);
  - a rotina que o relógio disparou só volta a disparar pelo relógio depois de o vigia vê-lo não
    pronto (um run que não replantou daria laço);
  - um run que o servidor viu começar e nunca terminar (o pod caiu no meio) conta como falho.
- **API:** `GET`, `POST` e `DELETE /api/microbot/recurring`, sempre com `?account=`. Salvar e apagar
  vão para a auditoria, e o run que uma recorrente começa entra nela com o ator `recurring:<id>`.
- **Sem tela:** a fila das recorrentes não está desenhada no canvas (regra 11). A API já devolve o
  que a tela vai mostrar: se vence, por quê, quando, e o que o vigia espera.
- **Fora desta entrega:**
  - Loadout com banco antes do run (o PREPARO): pede decisões do dono sobre o que sacar, e a
    verificação de cada plugin (se ele faz o próprio banco). Até lá, o plugin do run sai do banco
    sozinho ou o inventário já está pronto.
  - Requisitos da conta (Farming, Bone Voyage): o planner não confere; o dono liga a rotina quando a
    conta tiver. O relógio não vence para canteiro que nunca foi plantado.
  - Break próximo: feito depois, em "A troca", acima (o vigia espera o break).
  - Sucesso por estado observado (o Time Tracking mostrar os canteiros plantados de novo): por ora,
    só a trava do relógio.
- **Verificado:**
  - testes do agent com relógios falsos, todos com `-race`: vence, não vence, prioridade, um por
    vez, nada durante um run, intervalo desde o fim do último, relógio desconhecido cai no
    intervalo, travas, ponto seguro e arquivo;
  - store e rotas no Postgres 17, incluindo um agent da F6 que não recebe o campo novo e o oidc só
    com conta do dono;
  - `goose up/down/up` da 0015.
- **Nada foi testado no jogo.** Falta ligar uma recorrente no zaros, com o dono olhando, quando a
  conta tiver os requisitos.

Fora das fases 1 a 3, ainda: rotinas (fase 4), o serviço systemd e
o token via sops (fase 5) e a detecção de input humano: o Agent Server não diz se o dono está mexendo no
jogo, então o cartão "Input humano" do artboard não existe na tela.

## Impacto nos outros docs

- Doc 06: adendo apontando para este doc; o plugin de tracking continua somente leitura.
- Doc 04, seção 9: as fases acima entram depois do marco "em uso no zaros" (item 10), sem
  bloquear o caminho crítico do planner.
