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

```
navegador ──HTTPS/sessão──> ironman serve ──UDS + token──> Agent Server (cliente Microbot no Xvfb)
   │                            │
   │  noVNC (websocket) ────────┤──TCP 127.0.0.1:5999──> x11vnc ──> Xvfb :99
   │                            │
   │                            └──exec──> mb-client start|stop --headless (dono do processo do jogo)
```

- **`internal/microbot`**: client Go tipado do Agent Server (HTTP sobre o socket Unix, token lido do
  arquivo a cada requisição, porque muda quando o cliente reinicia). Sem dependência de banco.
- **Ciclo de vida do cliente**: o planner não reimplementa o launch. Ele chama o `mb-client` do
  harness (caminho em `MICROBOT_HARNESS_DIR`), que já cuida de JDK, hash do jar, HWID agent,
  `user.home` isolado, Xvfb e VNC. Uma fonte de verdade para como o jogo sobe.
- **VNC no navegador**: o servidor faz proxy de websocket para TCP (`/api/microbot/vnc` ↔
  `127.0.0.1:5999`), no lugar do `websockify`; a SPA usa o noVNC (`@novnc/novnc`).
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

Quem controla este módulo controla a conta do jogo e executa código no cliente.

- Desligado por padrão (`MICROBOT_CONTROL=off`); rotas `/api/microbot/*` só existem quando ligado.
- Exige sessão autenticada, inclusive o websocket do VNC. Em `AUTH_MODE=dev` o servidor só pode
  escutar em loopback ou na interface do Tailscale.
- O token do Agent Server e o VNC nunca saem da máquina; o navegador só fala com o planner.
- Rotas que alteram estado exigem proteção contra CSRF (sessão em cookie).
- Auditoria de toda ação de controle e de toda execução de rotina (`run_event`).

## Lacunas conhecidas

- **Esquema de configuração dos plugins**: o Agent Server só lê e grava chave/valor; não lista as
  opções de um plugin (nome, tipo, valores, seções). V1: edição chave/valor crua mais presets salvos
  por plugin. V2: endpoint de descritores de config (`ConfigDescriptor`), de preferência como PR
  upstream no Agent Server, ou num plugin companheiro no `microbot-plugins`.
- **Deploy no saradomin** (doc 04, seção 10): o cliente roda no zaros. Lá, o `serve` não alcança o
  socket local; precisará de uma ponte no zaros (`ironman bridge`) com conexão de saída para o
  servidor. Fora do escopo até o deploy.
- **GPU no modo headless**: desligada. Uma saída headless do Hyprland com `wayvnc` daria GPU real,
  mas depende da sessão gráfica aberta; fica como modo opcional futuro.

## Fases

1. `internal/microbot` (client do Agent Server), status do cliente, iniciar/parar via `mb-client`,
   login/logout, screenshot; tela **Microbot** com o toggle e o modo só dashboard.
2. VNC ao vivo: proxy websocket no `serve` e noVNC na SPA.
3. Plugins: listar, instalar do Hub, iniciar/parar, status/saúde/resultados, snapshot da state
   machine, config chave/valor.
4. Rotinas: modelo, executor com lock, agendamento, histórico com screenshots.
5. Ponte remota para o saradomin.

## Impacto nos outros docs

- Doc 06: adendo apontando para este doc; o plugin de tracking continua somente leitura.
- Doc 04, seção 10: as fases acima entram depois do marco "em uso no zaros" (tarefa 5), sem
  bloquear o caminho crítico do planner.
