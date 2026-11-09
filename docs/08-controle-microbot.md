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

## O que já existe (repositório `rune/microbot-plugins`)

O harness de desenvolvimento de plugins já resolve o lado do cliente:

| Peça | O que faz |
|---|---|
| `mb-client start --headless` | Sobe o cliente fixado (versão e hash em `microbot.json`) num display próprio `Xvfb :99`, com home isolada (`~/.local/share/microbot-dev`), HWID agent e Agent Server ligado. GPU e 117 HD ficam desligados (Xvfb não tem GPU). |
| VNC | `x11vnc` só em `127.0.0.1:5999`, sem senha; quem expõe (o planner) autentica. |
| Agent Server | API HTTP do próprio Microbot num socket Unix (`~/.local/share/microbot-dev/.runelite/agent/agent.sock`), com token em `.agent-token`. |
| `mb-account` | Escolhe o personagem da Jagex Account com que o cliente abre. |

Por que headless: uma janela do cliente escondida ou minimizada no desktop cai para cerca de 1 quadro
por segundo, e tudo (caminhar, clicar, esperar) fica lento. Medido com o personagem logado: chamadas
ao Agent Server em ~1000 ms com a janela escondida contra ~18 ms no Xvfb.

Endpoints do Agent Server que o módulo usa (referência completa em
`microbot-plugins/vendor/microbot/docs/AGENT_SERVER.md`):

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
`zaros`, que é o mesmo binário (`ironman agent`) rodando como serviço systemd do usuário, declarado no
`nix-config` ao lado do módulo do Microbot.

```
navegador ──Zitadel (OIDC) + papel──> ironman serve (saradomin; ou zaros no modo local-first)
                                            ▲
                                            │ websocket de SAÍDA, dentro do Tailscale,
                                            │ machine user do Zitadel; carrega comandos, eventos e o VNC
                                            │
                                 ironman agent (zaros, serviço systemd do usuário)
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
  harness (caminho em `MICROBOT_HARNESS_DIR`), que já cuida de JDK, hash do jar, HWID agent,
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
6. **Controle local**: `systemctl --user stop ironman-agent` (ou `ironman agent pause`) corta tudo
   no `zaros`. O agent não inicia ações enquanto o Microbot indicar input humano (o dono jogando).
7. **Web**: sessão exigida também no websocket do VNC, checagem de `Origin`, proteção contra CSRF
   nas rotas que alteram estado.
8. **Segredos fora do navegador**: o token do Agent Server e o VNC nunca saem do `zaros`; o
   navegador só fala com o `serve`.
9. **Auditoria**: toda ação de controle e toda execução de rotina (`run_event`), com usuário, origem
   e resultado.

## Lacunas conhecidas

- **Esquema de configuração dos plugins** (resolvido em 2026-11-08 pelo Dev Bootstrap 1.1.0, ver
  "Configuração com formulário"): o Agent Server só lê e grava chave/valor; não lista as
  opções de um plugin (nome, tipo, valores, seções). V1: edição chave/valor crua mais presets salvos
  por plugin. V2: endpoint de descritores de config (`ConfigDescriptor`), de preferência como PR
  upstream no Agent Server, ou num plugin companheiro no `microbot-plugins`.
- **GPU no modo headless**: desligada. Uma saída headless do Hyprland com `wayvnc` daria GPU real,
  mas depende da sessão gráfica aberta; fica como modo opcional futuro.

## Fases

1. `internal/microbot` (client do Agent Server) e `ironman agent` com a lista fechada de comandos,
   conectando no `serve` local; status do cliente, iniciar/parar via `mb-client`, login/logout,
   screenshot; tela **Microbot** com o toggle e o modo só dashboard; papel e auditoria.
2. VNC ao vivo: fluxo pela conexão do agent e noVNC na SPA.
3. Plugins: listar, instalar do Hub, iniciar/parar, status/saúde/resultados, snapshot da state
   machine, config chave/valor.
4. Rotinas: modelo, executor com lock, agendamento, histórico com screenshots.
5. Agent remoto: serviço systemd no `nix-config`, machine user do Zitadel com o segredo via sops, `WhoIs` do Tailscale e
   ACL; junto com o deploy do planner no `saradomin`. Como o agent existe desde a fase 1, aqui muda
   só configuração.

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
  (`nix develop $MICROBOT_HARNESS_DIR --command .../bin/mb-client`, ou `MICROBOT_CLIENT_CMD`);
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
  `MICROBOT_HARNESS_DIR` assume o checkout irmão `../microbot-plugins`). Contra um `serve` em oidc:
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

- `microbot-plugins`, Dev Bootstrap 1.1.0: escreve `.runelite/agent/config-schema.json` com as
  opções de cada plugin do Microbot carregado (grupo, seções, itens com tipo, intervalo, unidade,
  nomes e rótulos de enum e o aviso do plugin), sem os itens `secret`, e reescreve quando um plugin
  carrega, descarrega ou muda. Os valores não vão no arquivo: continuam no `/settings/plugin`.
- `config_schema` (leitura): o agent lê o arquivo e devolve o formulário de um plugin, com as regras
  do `config_get` (sem grupo protegido, sem chave que pareça segredo). `GET
  /api/microbot/config/schema?className=`.
- Web, em Plugins: o formulário por seção (as fechadas por padrão começam fechadas), com o controle
  do tipo (caixa para booleano, número com mínimo, máximo e unidade, lista para enum, texto para o
  resto, amostra de cor), "Gravar" por opção alterada (segundo clique quando o plugin tem aviso) e
  "padrão" para apagar a chave. Sem o arquivo (Dev Bootstrap antigo), a tela cai no chave/valor cru,
  que também fica a um clique como alternativa.
- Para ligar no zaros: `mb-install devbootstrap` (home de dev, nunca `--live`) e reiniciar o cliente.
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
- Entrega 2 (a ponte, feita): o Dev Bootstrap 1.2.0 do microbot-plugins serve um segundo socket,
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
- Para ligar no zaros: `mb-install devbootstrap` (home de dev) e reiniciar o cliente.

Fora das fases 1 a 3, ainda: rotinas (fase 4), o serviço systemd e
o token via sops (fase 5) e a detecção de input humano: o Agent Server não diz se o dono está mexendo no
jogo, então o cartão "Input humano" do artboard não existe na tela.

## Impacto nos outros docs

- Doc 06: adendo apontando para este doc; o plugin de tracking continua somente leitura.
- Doc 04, seção 9: as fases acima entram depois do marco "em uso no zaros" (item 10), sem
  bloquear o caminho crítico do planner.
