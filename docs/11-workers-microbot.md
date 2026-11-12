# 11 — Workers do Microbot: contas, perfis e clientes headless no saradomin

**Estado: rodada 1 implementada (2026-11-12) e falta ligar no saradomin; o que foi feito e conferido
está na seção 14. Perfis (F5), rotinas no servidor (F6), métricas (F7) e GPU continuam como
proposta.**
Substitui a primeira versão do mesmo dia. A primeira versão definiu o desenho do cluster (imagens,
RBAC, iGPU e medições), e isso continua valendo. A segunda versão parte de três respostas do dono:

1. **Pods criados pelo Chronicle**, pela web e pela CLI. Conectar uma conta grava a credencial dela
   no Postgres e a habilita para ser gerenciada.
2. **Estado do cliente no Postgres:** plugins, configurações e perfis passam a ser dados do Chronicle
   e deixam de morar no disco da máquina do jogo.
3. **Qualquer conta**, com ou sem rota. Mais adiante entram rotas próprias e simples, como construir
   uma conta do zero até um money making, de um jeito reproduzível, para ter vários gold farmers.
   Rotas e objetivos vão ser revistos depois (seção 11).

Registro: doc 01, decisão 34. Sequência: doc 04, seção 9, item 19. O protótipo da tela está no
canvas, página 7 (`V6MicrobotWorkers1440`, `V6MicrobotWorkersEstados1440`). Ele ainda não tem os
estados novos deste doc (conectar conta, perfil, desvio de config); esses estados precisam ser
desenhados antes do código (regra 11 do CLAUDE.md).

## 1. A ideia em uma frase

Cada **conta** do Chronicle pode ter um **worker**: um cliente Microbot headless controlado por um
`chronicle agent`. O worker roda no zaros (anexado) ou num pod do saradomin (gerenciado) e sobe a
partir de três dados do banco: a **sessão** da conta, o **perfil** do cliente e as **rotinas**. A
web e a CLI mostram e controlam todos os workers por conta.

## 2. Vocabulário

Na primeira versão, "agent", "worker" e "cliente" se misturavam. Daqui em diante:

| Termo | O que é | Onde vive |
|---|---|---|
| **Conta** | Um personagem do jogo no Chronicle (`accounts`). Passa a aceitar contas sem rota e o modo `regular` (seção 4.1) | Postgres |
| **Sessão da conta** | O que o cliente precisa para entrar como aquele personagem sem o launcher: `JX_SESSION_ID`, `JX_CHARACTER_ID`, `JX_DISPLAY_NAME` e os arquivos de identidade de aparelho do HWID agent | Postgres, cifrada |
| **Perfil do cliente** | Plugins (do Hub e nossos, com versão), configuração dos plugins (grupo, chave, valor), perfil gráfico e mundo preferido. Um perfil pode servir de modelo para várias contas | Postgres |
| **Rotina** | Gatilhos, "fazer agora", recorrentes e o histórico de runs (doc 08, fase 4) | Postgres (hoje está em arquivos do agent) |
| **Worker** | Um cliente do jogo rodando para uma conta, num lugar | Processo ou pod |
| **Agent** | O `chronicle agent` que controla um worker. Um agent para um worker, com uma conta por vez | Ao lado do cliente |
| **Lugar** | `attached` (o zaros: o Chronicle controla o cliente, mas não o sobe nem o derruba) ou `managed` (um pod que o Chronicle cria e apaga) | Coluna do worker |

Regra de endereçamento: **tudo é endereçado pela conta**, nunca pelo agent. As rotas
`/api/microbot/*` ganham `?account=` e passam pelo par `Server.authed` + `Server.accountParam`
(regra 10 do CLAUDE.md), como as outras rotas por conta. O Hub sabe qual conexão serve cada conta. O
dono pensa em contas, e o lugar onde a conta roda é só um atributo dela.

## 3. Componentes

```
                      ┌──────────────── namespace chronicle (saradomin) ─────────────────┐
 navegador ──tailnet──► Ingress ─► chronicle serve                                       │
 CLI (zaros) ─────────►            ├─ Hub: conta → conexão do agent                      │
                      │            ├─ cofre de sessões (AES-256-GCM, chave no Secret)    │
                      │            ├─ perfis e rotinas (Postgres)                         │
                      │            ├─ reconciliador de workers ──API k8s──┐              │
                      │            └─ Postgres (CNPG, ns databases)       │              │
                      │                  ▲ websocket (token projetado)    ▼              │
                      │            worker-<conta> (Deployment, 1 réplica, Recreate)      │
                      │            └─ chronicle agent --worker                           │
                      │               ├─ Xvfb + x11vnc + VirtualGL ─► /dev/dri (iGPU)    │
                      │               └─ cliente Microbot (Chronicle Link, Agent Server) │
                      └──────────────────────────────────────────────────────────────────┘
 zaros: chronicle agent (anexado) ──websocket (usuário de máquina do Zitadel)──► mesmo Hub
```

| Peça | Repositório | Muda o quê |
|---|---|---|
| `chronicle serve` | chronicle | Hub por conta, cofre, perfis, rotinas no banco, reconciliador, rotas `/api/workers` e `/api/accounts/{id}/session` |
| `chronicle agent` | chronicle | Modo `--worker` (bootstrap pelo link). Nos dois modos, recebe perfil e rotinas do servidor e devolve os eventos |
| `chronicle accounts connect`, `chronicle workers …` | chronicle | CLI de conectar conta e de iniciar e parar workers |
| Imagem `microbot-worker-base` | microbot-plugins | Runtime do jogo pelo Nix, cliente fixado e nossos plugins |
| Imagem `chronicle-worker` | chronicle | A base mais o `chronicle` do mesmo commit do servidor |
| RBAC, quota, GPU, rede | gielinor | O que o cluster precisa para o Chronicle criar workers com segurança |

## 4. Modelo de dados

Uma migração por fase (seção 10). Os formatos finais ficam nas migrações. Os nomes abaixo são a
proposta.

### 4.1 Contas: qualquer personagem

`accounts` hoje exige `mode IN ('ironman','hcim','uim','gim')` e `guide_id NOT NULL`. Muda assim:

- `mode` ganha `regular`;
- `guide_id` passa a aceitar nulo: a conta fica "sem rota", e o Agora e as Rotas mostram o estado
  vazio em vez de quebrar (precisa de desenho no canvas);
- `managed BOOLEAN NOT NULL DEFAULT false`: a conta pode ter um worker gerenciado. Só pode ser `true`
  quando existe uma sessão.

O tracking (Chronicle Link, hiscores) funciona igual numa conta sem rota: os fatos e o `player_state`
não dependem de rota. Uma conta ainda não detectada nos hiscores continua válida.

### 4.2 Sessões

```
game_sessions   account_id PK → accounts, character_name, ciphertext, nonce, key_id,
                pushed_at, pushed_by, last_login_at, last_login_result
```

- O conteúdo cifrado é um JSON com as três variáveis `JX_*` e os arquivos de aparelho:
  `spoofed-device-ids.properties` e `spoofed-random-dats.properties`, **só as entradas daquele
  personagem**, para a identidade do aparelho continuar a mesma entre o zaros e o pod.
- O servidor cifra com `CHRONICLE_VAULT_KEY` (AES-256-GCM, `key_id` para rotacionar). A API nunca
  devolve a sessão; a sessão só sai pelo link, para o agent daquela conta.
- O painel mostra o personagem, a data da conexão e o resultado do último login.

### 4.3 Perfis do cliente

```
client_profiles          id, user_id, name, parent_id (opcional, modelo), created_at, updated_at
client_profile_plugins   profile_id, source (hub | own | core), name (internalName ou classe),
                         version (fixa ou nula = a da imagem/Hub), enabled, start_on_boot
client_profile_settings  profile_id, config_group, config_key, value
account_client           account_id PK, profile_id, overrides (jsonb: grupo.chave → valor),
                         world, graphics (enum: minimal | default)
```

- **Modelo e herança:** o perfil efetivo de uma conta é o do modelo (`parent_id`), mais o próprio
  perfil, mais os `overrides` da conta. É isso que torna um "gold farmer" reproduzível: um perfil
  modelo com os plugins e as configurações do método, aplicado a N contas.
- **Perfil gráfico** (seção 7.3) vira um perfil de sistema (`graphics = minimal`), mesclado por baixo
  de tudo, então o dono não precisa repetir em cada conta.
- **O que nunca entra:** os grupos e chaves que o `config_set` já recusa (`agentServer`, `runelite`,
  `microbot`, `fm-ironmanplanner`, chaves que parecem segredo; `agent.ValidateParams`). A mesma
  validação vale ao gravar no banco.
- **Fonte da verdade:** num worker gerenciado, o banco. Uma configuração gravada pelo Cockpit vai
  primeiro para o banco e depois para o cliente. Uma mudança feita direto no painel do RuneLite pelo
  VNC vira **desvio**: o Chronicle Link já exporta o esquema das opções (`config-schema.json`) e passa
  a exportar também os valores que não são segredo. O agent compara com o perfil efetivo, e a tela
  mostra "3 opções diferentes do perfil" com "Salvar no perfil" ou "Descartar". Gravar sozinho
  ficaria escondido do dono (W9).
- **Anexado (zaros):** o perfil é opcional. Sem perfil, o zaros continua como hoje, com a config no
  disco. Com perfil, o agent aplica só o que o perfil declara.

### 4.4 Rotinas e runs

Os gatilhos (`agent-triggers.json`) e o histórico (`agent-runs.json`) saem do disco do agent:

```
routines       id, account_id, kind (now | trigger | recurring), spec (jsonb: os parâmetros
               do run_start), event (ex.: kitten_grown), enabled, created_at
routine_runs   id, account_id, routine_id, agent_id, trigger, started_at, finished_at,
               status, outcome (jsonb)
run_events     run_id, seq, at, state, kind, detail (jsonb)
```

- O executor continua no agent (decisão R1 do doc 08): o agent recebe as rotinas da conta no
  bootstrap e a cada mudança, roda e devolve os eventos.
- O link ganha mensagens **do agent para o servidor** (`event`), com fila em memória enquanto o link
  está caído. O `serve` grava `run_events`, e a auditoria das ações que o executor faz sozinho (hoje
  ela só registra o começar e o cancelar) vem pelo mesmo canal.
- O arquivo local continua como cache do anexado, para o gatilho valer com o `serve` fora. Num pod,
  ele vive em memória.

### 4.5 Workers

```
workers   id, account_id UNIQUE, placement (attached | managed), desired_state (running |
          stopped), observed_state, image, agent_version, started_at, stopped_at, last_error
```

- Uma conta tem um worker, e só uma conta pode estar ativa num lugar. O banco garante: `UNIQUE
  (account_id)`, mais a trava do Hub (seção 6.2).
- `microbot_audit` ganha `account_id` e `agent_id`.

### 4.6 O que não vai para o Postgres

O **cache do jogo** (centenas de MB de arquivos do cliente) não faz sentido no banco. Ele vai para
um **PVC compartilhado** por todos os workers (W5, decidido pelo dono em 2026-11-12), descrito na
seção 7.4. Os **jars dos plugins** vêm da imagem (os nossos) ou do Hub (baixados no boot). O banco
guarda qual plugin e qual versão, não o binário.

## 5. Conectar uma conta

É o "habilitar para gerenciar". A sessão só existe na máquina onde roda o launcher da Jagex, então a
captura é sempre pela CLI. O resto pode ser feito pela web.

```
chronicle accounts connect --character EventSourced [--account acc_x] [--profile farmer-base]
```

1. Lê do `~/.microbot/accounts.json` **só** a entrada daquele personagem e, dos arquivos de aparelho,
   só as linhas dele.
2. Sem `--account`, cria a conta (`regular`, sem rota, rsn = nome do personagem) ou reaproveita a do
   mesmo rsn do usuário.
3. `PUT /api/accounts/{id}/session` com o bearer do login da CLI. O servidor cifra e grava, e marca
   `managed = true`.
4. Opcional: associa um perfil.

Na web, a Conta ganha a seção **Automação da conta**: estado da sessão, perfil, lugar e o comando
de conectar já preenchido, para copiar quando a sessão falta ou expira. Reimplementar o login da
Jagex continua fora do escopo.

Sessão expirada: o worker sobe, o login falha e `last_login_result` registra o motivo. O painel diz
"abra o launcher no zaros e rode o connect de novo".

## 6. Ciclo de vida

### 6.1 Iniciar

Pela web (Automação › Workers, "Iniciar") ou pela CLI (`chronicle workers start acc_x`), as duas sobre
`POST /api/workers/{conta}/start`:

1. **Trava** (seção 6.2).
2. `desired_state = running`.
3. O reconciliador cria `worker-<conta>`:
   - Deployment com `replicas: 1` e **`strategy: Recreate`**: um RollingUpdate subiria o pod novo antes
     de derrubar o velho, e seriam dois clientes na mesma conta;
   - imagem `WORKER_IMAGE` do configMap;
   - `requests` de 500m e 1 GiB, `limits` de 1 núcleo e 1,5 GiB, `gpu.intel.com/i915: 1`;
   - ServiceAccount `chronicle-worker`, com token projetado de audiência `chronicle`;
   - `emptyDir` em memória para a home e o PVC `microbot-game-cache` montado só no diretório do
     cache do jogo (seção 7.4);
   - `CHRONICLE_URL=http://chronicle.chronicle.svc:8080`.

### 6.2 Uma conta num lugar só

O servidor recusa o Iniciar quando:

- outro agent já serve a conta (o anexado informa a conta no hello e a cada `status`);
- a conta tem worker em `running`;
- **o Chronicle Link de outra instalação mandou um push dessa conta nos últimos 5 minutos.** O dono
  está jogando no cliente de todo dia, e entrar no pod derrubaria ele. O ingest já registra a
  instalação de cada push (`plugin_ingest_tokens`).

### 6.3 Boot do worker

1. O `chronicle agent --worker` conecta ao Hub com o token projetado. O servidor valida pela
   TokenReview e identifica o pod pelo nome no token.
2. Hello: worker, conta, `kind=managed` e versão.
3. O servidor manda pelo link o pacote de bootstrap:
   - a sessão decifrada;
   - um token `ipl_` novo para o Chronicle Link, com escopo daquela conta, revogado no Parar. O
     device flow é para pessoas; o `ipl_` continua aceito pelo código em todos os modos e serve de
     credencial de máquina por worker;
   - o perfil efetivo (plugins, configurações, gráfico, mundo);
   - as rotinas da conta.
4. O agent escreve na home em memória: `credentials.properties`, os arquivos de aparelho, o perfil do
   RuneLite com as configurações (W7), e a config do Chronicle Link com a URL interna e o token.
5. `mb-client start --headless` com `MB_HEADLESS_GPU=1` e `MB_CLIENT_WRAPPER="vglrun -d egl"`.
6. Com o Agent Server de pé, o agent instala os plugins do Hub do perfil e liga os `start_on_boot`.
7. O cliente fica na tela de login. Entrar é outro clique, com o mundo (W3).

### 6.4 Uso

É o que o doc 08 já faz (Cockpit, VNC pelo túnel, plugins, Fazer agora, gatilhos), agora com
`?account=`. O Cockpit ganha o seletor de conta (artboard dos estados da página 7). O Chronicle Link
do worker alimenta o tracking da conta pela URL interna, como qualquer instalação.

### 6.5 Parar, reiniciar, atualizar e falhas

- **Parar** (web, ou `chronicle workers stop acc_x`): o agent faz logout limpo e para o cliente. O
  reconciliador apaga o Deployment, e o token `ipl_` do worker é revogado. A sessão some com a
  memória do pod; perfil e rotinas ficam no banco.
- **Atualizar:** depois de um deploy, os workers aparecem como "versão antiga". Reiniciar desloga a
  conta, então só acontece no clique ou com o worker deslogado. O protocolo do link continua aditivo.
- **Falhas:**
  - **pod cai:** o Deployment o recria; o agent pede o bootstrap de novo e fica na tela de login. Um
    run em andamento vira `failed` ("o worker reiniciou no meio do run");
  - **link cai:** o agent segue o run e enfileira os eventos;
  - **`serve` reinicia:** os agents reconectam, e o reconciliador compara `desired_state` com os
    Deployments.

## 7. Cluster (gielinor)

Valem as regras da primeira versão, com dois acréscimos:

- **Namespace** `chronicle` para o web e os workers. O Argo não apaga os workers, porque eles não
  têm a etiqueta dele.
- **SA `chronicle`:** um Role com create, delete, get, list, watch e patch em `deployments`, e get,
  list e watch em `pods`. Nenhuma permissão em secrets. `system:auth-delegator` para a TokenReview.
- **ValidatingAdmissionPolicy** para pedidos da SA `chronicle` em Deployments: etiqueta
  `component: worker`, SA `chronicle-worker`, imagem `ghcr.io/felipemalacarne/chronicle-worker`,
  **`replicas <= 1` e `strategy.type == Recreate`** (acréscimo), sem `hostPath`, sem `privileged`,
  volumes só `emptyDir`, o token projetado e o PVC `microbot-game-cache` (seção 7.4).
- **SA `chronicle-worker`:** sem permissão nenhuma e com `automountServiceAccountToken: false`, só o
  token projetado.
- **ResourceQuota** no namespace. Um worker que não cabe fica Pending, e o painel mostra.
- **iGPU compartilhada:** `-shared-dev-num=4` no `intel-gpu-plugin` (hoje o Jellyfin tem o único
  slot).
- **NetworkPolicy:** os workers saem só para o Service do chronicle e para a internet.
- **Segredo:** `CHRONICLE_VAULT_KEY` no `chronicle-secrets` (SOPS).
- **CPU e RAM por worker no painel:** o agent lê o próprio cgroup e manda pelo link (acréscimo).
  Assim o servidor não precisa de permissão no `metrics.k8s.io`.

### 7.1 Medições (N95, 2026-11-12)

4 núcleos e 16 GB; livres, cerca de 3,4 núcleos e 7,7 GB. Cliente 2.6.52 em 2 núcleos, logado e
parado, FPS 20:

| Modo | CPU | RSS |
|---|---|---|
| GPU na iGPU (Xvfb + VirtualGL `-d egl`) | 0,36 núcleo | 853 MiB |
| Software | 0,43 núcleo | 808 MiB |

O limite é a RAM: cabem 3 a 4 workers ao lado do resto do cluster. Ainda não foram medidos: uma cena
cheia, um script rodando e o `/observe` do Cockpit. Uma frota de gold farmers maior que isso precisa
de outro nó (W8).

### 7.2 Imagens e CI

Sem mudança em relação à primeira versão:

- `microbot-worker-base` pelo Nix (`dockerTools`), do mesmo `flake.lock` do harness: JDK 21, Xvfb,
  x11vnc, VirtualGL, Mesa, os jars de `microbot.json`, os nossos plugins e o `bin/` do harness.
  Usuário 65532, tag `<versão do Microbot>-<sha7>`. O CI novo no `microbot-plugins` roda
  `gradle test` e, na `main`, publica a imagem.
- `chronicle-worker`: `FROM` a base fixada por digest em `deploy/worker/base.lock`, mais o binário do
  mesmo commit. O job `docker` publica as duas imagens e grava `WORKER_IMAGE` no overlay.
- O bump da base no chronicle é manual: `just worker-base-bump <tag>` (W4).

### 7.3 Perfil gráfico `minimal`

Plugin GPU ligado com draw distance 15–20, MSAA desligado, anisotrópico 0 e sem `unlockFps`; 117 HD
desligado; FPS Control em 15–20; Entity Hider escondendo outros jogadores e os pets deles; Low Detail
ligado; display de 1024×768.

### 7.4 Cache do jogo compartilhado

Um PVC só, `microbot-game-cache`, para todos os workers. Ele é declarado no gielinor (o Chronicle só
o referencia, então o RBAC não muda) e usa o `local-path`, como o do filebrowser:

- **RWO basta:** o `ReadWriteOnce` do `local-path` é por nó, não por pod, então todos os workers do
  saradomin montam o mesmo volume. Um segundo nó teria o próprio PVC; o cache é descartável.
- **Só o cache:** monta-se só o diretório do cache do jogo (`.runelite/jagexcache`, a confirmar no
  spike da F3), nunca a home, o perfil do RuneLite ou os plugins do Hub, que são por conta.
- **Nada que identifique o aparelho:** se o `random.dat` ou qualquer arquivo de identidade cair
  nesse diretório, ele não pode ser compartilhado. Um `random.dat` comum ligaria todas as contas
  entre si. Na F3, conferir onde o cliente o grava e que o HWID agent o intercepta por conta
  (`microbot.hwid.randomdat.store`). Se ele estiver no cache, o volume monta só os arquivos do cache
  (`subPath`).
- **Vários clientes ao mesmo tempo:** é o caso normal de quem roda dois clientes num mesmo PC, que
  dividem o `jagexcache`. O risco fica no dia de update do jogo, com dois clientes baixando a mesma
  atualização. O agent segura um `flock` em `cache.lock` no volume do início do cliente até a tela de
  login, então os boots acontecem um de cada vez (cerca de 40 s cada).
- **Admissão:** a ValidatingAdmissionPolicy aceita nos workers só `emptyDir`, o token projetado e
  esse PVC pelo nome.
- **Tamanho:** pedido de 2 GiB, para sobrar espaço depois dos updates. O pod roda como 65532, com
  `fsGroup` para escrever no volume.

## 8. API e CLI

| Rota | O quê |
|---|---|
| `GET /api/workers` | Workers das contas do usuário: lugar, estado, versão, CPU e RAM, sessão |
| `POST /api/workers/{conta}/start` \| `stop` \| `restart` | Ciclo de vida, auditado |
| `GET` \| `PUT /api/accounts/{id}/session` | Estado da sessão (sem o conteúdo) e conexão |
| `GET` \| `POST` \| `PATCH /api/client-profiles…` | Perfis, plugins, configurações e modelo |
| `GET` \| `PUT /api/accounts/{id}/client` | Perfil da conta, overrides, mundo e gráfico; desvio atual |
| `GET` \| `POST /api/accounts/{id}/routines`, `GET …/runs` | Rotinas e histórico no banco |
| `/api/microbot/*?account=` | O que existe hoje, endereçado pela conta |

CLI: `chronicle accounts connect`, `chronicle workers list|start|stop|restart`,
`chronicle profiles list|show|apply`. Nenhuma abre o banco (regra 1 do CLAUDE.md).

## 9. Segurança e riscos

- O módulo continua atrás de `MICROBOT_CONTROL=on` e do papel `microbot:control`. A lista fechada de
  comandos continua sendo a camada principal: o bootstrap entra nela como comando novo, e o agent
  nunca executa nada que venha cru do servidor.
- A sessão só existe decifrada em dois lugares: na memória do `serve` durante o bootstrap e na
  memória do pod.
- **Contas ligadas entre si:** mesma casa, mesmo IP e mesmo dono. Uma ação da Jagex contra uma conta
  tende a pegar as outras. Com vários gold farmers, esse risco cresce com cada conta (W8).
- **GPU sem isolamento:** um worker travado na GPU afeta o Jellyfin e os outros workers.

## 10. Fases (execução)

A ordem muda em relação à primeira versão: o cofre vem **antes** do worker de teste, então o Secret
SOPS provisório deixa de existir. E as rotinas no banco viram fase própria, porque um pod perde o
disco.

| Fase | Entrega | Pronto quando | Onde |
|---|---|---|---|
| F0 | Link remoto: `MICROBOT_CONTROL=on` no saradomin, machine user do Zitadel para o agent do zaros, serviço no zaros (doc 08, fase 5) | o Cockpit do saradomin controla o cliente do zaros | gielinor, nix-config |
| F1 | Hub por conta: hello com conta, kind, worker e a versão da ponte do Chronicle Link (o agent exige uma mínima e diz qual falta, decisão 35 do doc 01); rotas com `?account=`; mensagens `event` do agent; `microbot_audit.account_id`/`agent_id`; Automação › Workers com o zaros na lista | duas contas no mesmo servidor, cada uma com seu agent (o segundo pode ser um agent de teste no zaros) | chronicle |
| F2 | Contas e sessões: migração (`regular`, rota opcional, `managed`, `game_sessions`), cofre, `chronicle accounts connect`, Automação da conta na Conta | uma conta sem rota conectada pela CLI; a sessão aparece na web e nunca volta pela API | chronicle |
| F3 | Imagens e um worker aplicado à mão: as duas imagens e os dois CIs, iGPU compartilhada, o PVC do cache, um Deployment escrito à mão que faz o bootstrap pelo link (sessão do cofre, `ipl_` do worker, perfil `minimal` fixo) | o dono entra no jogo pelo VNC do pod, o Chronicle Link dele alimenta o tracking; medir de novo com o perfil enxuto | microbot-plugins, chronicle, gielinor |
| F4 | Gerenciado: reconciliador, cliente REST do k8s (W1), RBAC e admission policy, Iniciar, Parar e Reiniciar pela web e pela CLI, trava da seção 6.2 | Iniciar e Parar sem `kubectl`; um pod apagado à mão volta para a tela de login | chronicle, gielinor |
| F5 | Perfis no Postgres: tabelas da seção 4.3, aplicar no boot, plugins do Hub do perfil, desvio e "Salvar no perfil", modelos | uma segunda conta sobe com o mesmo modelo e fica com os mesmos plugins e configurações | chronicle, microbot-plugins (export dos valores) |
| F6 | Rotinas no servidor: seção 4.4, gatilhos e histórico saindo dos arquivos, auditoria das ações do executor | o gatilho do gato sobrevive a um restart do pod; o histórico aparece no banco | chronicle |
| F7 | Capacidade e métricas: "cabe mais N", CPU e RAM por worker, aviso de sessão expirando, reinício na versão nova | por entrega | chronicle |

Antes de cada fase com tela (F1, F2, F5, F6), o desenho entra no canvas e o dono aprova (regra 11).
Até a F5, um pod gerenciado que reinicia perde a configuração dos plugins. Para testar, isso é
aceitável.

## 11. O que fica para a revisão de rotas e objetivos

O dono quer rotas próprias e mais simples para contas novas (do zero até um money making) e uma
forma reproduzível de construir várias contas. Este doc só prepara o terreno:

- conta sem rota (seção 4.1);
- perfil modelo (seção 4.3): o "como o cliente está montado" de uma conta de farm;
- rotinas no banco por conta (seção 4.4): o "o que ela roda".

O que falta para uma **receita de conta** completa (perfil modelo + plano + rotinas + metas de saída)
depende dos planos customizados e das receitas por objetivo do doc 10 (seções 8 e 9, decisões E5 a
E10). A revisão de rotas e objetivos que o dono quer fazer depois é o lugar disso, não este doc.

## 12. Decisões abertas

| # | Decisão | Recomendação |
|---|---|---|
| W1 | `client-go` ou um cliente REST mínimo para a API do k8s | REST mínimo (Deployments e TokenReview) |
| W2 | Base pelo Nix ou Dockerfile Debian | Nix: o mesmo pin do harness |
| W3 | O worker entra no jogo sozinho ao subir? | Não; no máximo uma opção por conta, desligada |
| W4 | Bump da base manual ou automático | Manual (`just worker-base-bump`) |
| W5 | Cache do jogo por pod ou compartilhado | **Resolvida (2026-11-12):** PVC compartilhado (seção 7.4) |
| W6 | Lista de workers antes dos gerenciados | **Resolvida** pela F1 |
| W7 | Como aplicar a config do perfil: escrever o arquivo de perfil do RuneLite antes de o cliente subir, ou `/settings/plugin` depois | Arquivo antes (os plugins já leem a config certa no start); spike na F3 |
| W8 | IP e escala de uma frota: proxy por worker, outro nó, outra casa | Adiar até haver mais de 3 contas; registrar o risco |
| W9 | Desvio de config: salvar sozinho ou pelo botão | Pelo botão ("Salvar no perfil") |
| W10 | Conta sem rota: `guide_id` nulo e modo `regular` | Sim, na migração da F2 |

## 13. Contratos da implementação (rodada 1, 2026-11-12)

Fixados antes de dividir o trabalho, para as frentes andarem em paralelo. Fora desta rodada: GPU e
VirtualGL (o pod renderiza por software: 0,43 contra 0,36 núcleo medidos, e a iGPU do Jellyfin fica
como está), perfis e rotinas no Postgres (F5 e F6), conta sem rota e métricas (F7). A tela segue a
estrutura da página 7 do canvas sem polimento visual, por decisão do dono (2026-11-12: "tudo
funcional primeiro, a aparência depois").

### 13.1 Protocolo do link

Em `internal/agent/protocol.go` e `internal/agent/worker_protocol.go`:

- `Hello` ganha `account`, `kind` (`attached` | `managed`; vazio é `attached`), `worker` (nome do pod)
  e `bridge` (versão da ponte do Chronicle Link, vazia sem ponte).
- Comando `worker_bootstrap` (`BootstrapParams`: conta, `GameSession`, `plugin_token`, `server_url`,
  `world`; resposta `BootstrapResult`). Só o agent em `--worker` executa; ninguém registra os
  parâmetros (log ou auditoria), porque levam a sessão.
- Roteamento: o servidor escolhe o agent pela conta (`?account=`). Sem `?account=` e com um agent só,
  usa esse (compatível com a web de hoje). Um agent sem conta declarada só atende quando nenhum
  outro declara aquela conta. Um agent `managed` só pode declarar a conta do seu pod.

### 13.2 Variáveis

| Onde | Variável | O quê |
|---|---|---|
| `serve` | `WORKERS` (`off`) | `on` liga o reconciliador e `/api/workers`; exige `MICROBOT_CONTROL=on` e `CHRONICLE_VAULT_KEY` |
| `serve` | `CHRONICLE_VAULT_KEY` | 32 bytes em base64 (AES-256-GCM); o id da chave são os 8 primeiros hex do SHA-256 dela |
| `serve` | `WORKER_IMAGE` | imagem `chronicle-worker` (o CI grava no overlay) |
| `serve` | `WORKER_NAMESPACE` | padrão: o namespace do próprio pod (arquivo da SA) |
| `serve` | `WORKER_SERVER_URL` | padrão `http://chronicle.<namespace>.svc` (o Service do app, porta 80) |
| attached | `MICROBOT_AGENT_ACCOUNT` | conta que o agent do zaros declara no hello (opcional) |
| worker | `WORKER_ACCOUNT`, `CHRONICLE_URL`, `CHRONICLE_AGENT_TOKEN_FILE` | conta do pod, servidor, token projetado (relido a cada conexão) |
| worker | `MB_DEV_HOME`, `MB_XMX`, `MB_HWID_STORE_DIR`, `MB_PREBUILT_PLUGINS` | opções do `mb-client` (seção 13.4) |
| worker | `CHRONICLE_LINK_SERVER_URL`, `CHRONICLE_LINK_TOKEN` | o Chronicle Link usa estes no lugar da config quando presentes (credencial `ipl_`) |

### 13.3 O pod

- Deployment `worker-<conta>`: a conta em minúsculas, `_` vira `-` (`acc_iron_man` →
  `worker-acc-iron-man`). `replicas: 1`, `strategy: Recreate`, `revisionHistoryLimit: 1`.
- Etiquetas: `app.kubernetes.io/name: chronicle`, `app.kubernetes.io/component: worker`,
  `chronicle.ftm.dev.br/account: <conta>`. O Argo não gerencia esses objetos.
- `serviceAccountName: chronicle-worker`, `automountServiceAccountToken: false`, token projetado de
  audiência `chronicle` em `/var/run/secrets/chronicle/token` (3600 s).
- Segurança: `runAsUser`/`runAsGroup`/`fsGroup` 65532, `runAsNonRoot`, `seccompProfile:
  RuntimeDefault`, sem privilégios, `allowPrivilegeEscalation: false`, capabilities `drop: [ALL]`.
- Recursos: `requests` 500m e 1 GiB; `limits` 1 núcleo e 1792 MiB; `MB_XMX=1024m`.
- Volumes: `home` (`emptyDir` em disco) em `/home/worker`; `secrets` (`emptyDir` em memória, 1 MiB)
  em `/run/chronicle` para os arquivos do HWID agent; o PVC `microbot-game-cache` em
  `/home/worker/microbot-dev/.runelite/jagexcache`; o token projetado.
- A sessão da Jagex entra só como variável de ambiente do processo do cliente (`JX_SESSION_ID`,
  `JX_CHARACTER_ID`, `JX_DISPLAY_NAME`, como o launcher faz), nunca em arquivo.
- O agent segura `flock` em `jagexcache/.chronicle-boot.lock` do início do cliente até o Agent
  Server responder (seção 7.4).

### 13.4 Imagens

- `ghcr.io/felipemalacarne/microbot-worker-base:<versão do Microbot>-<sha7>` (microbot-plugins):
  - `/opt/microbot/bin`: o harness, no PATH;
  - o JDK 21, Xvfb, x11vnc, xdotool, bash, coreutils, util-linux, procps, jq, curl e fontes;
  - `MICROBOT_CLIENT_JAR` e `MICROBOT_HWID_AGENT_JAR` apontando para os jars fixados;
  - `MB_PREBUILT_PLUGINS=/opt/microbot/plugins` com os jars do `gradle pluginJars`;
  - usuário 65532, `HOME=/home/worker`, `MB_DEV_HOME=/home/worker/microbot-dev`.
- `mb-client` ganha as variáveis abaixo, e sem elas tudo continua como hoje:
  - `MB_XMX` (padrão `2g`);
  - `MB_HWID_STORE_DIR` (padrão `$HOME/.runelite`);
  - `MB_PREBUILT_PLUGINS`: instala os jars dali em vez de compilar o DevBootstrap e copia também o
    Chronicle Link;
  - pula o PipeWire e o `xdotool` quando não existem.
- `ghcr.io/felipemalacarne/chronicle-worker:<sha do chronicle>`: `FROM` a base fixada por digest em
  `deploy/worker/base.lock`, com o `chronicle` em `/usr/local/bin/chronicle` e
  `ENTRYPOINT ["chronicle", "agent", "--worker"]`.

### 13.5 Banco (migração 0012)

- `accounts.managed BOOLEAN NOT NULL DEFAULT false`.
- `game_sessions`: `account_id` PK (FK), `character_name`, `key_id`, `nonce` e `ciphertext` (`bytea`),
  `pushed_at`, `pushed_by`, `last_login_at`, `last_login_result`.
- `workers`: `account_id` PK (FK), `desired_state` (`running` | `stopped`), `world`, `image`,
  `plugin_token_id` (FK do token `ipl_` do worker), `started_at`, `stopped_at`, `updated_at`,
  `last_error`. Só os gerenciados; os anexados vivem no Hub.
- `microbot_audit` ganha `account_id` e `agent_id` (texto, vazio nas linhas antigas); o
  `storemicrobot.Entry` já tem os campos.

## 14. Rodada 1: o que foi feito (2026-11-12)

### 14.1 Entregue

**chronicle:**

- `internal/vault`: AES-256-GCM, chave de `CHRONICLE_VAULT_KEY`; a aad é a conta.
- Migração 0012:
  - `game_sessions` e `workers`;
  - `accounts.managed`;
  - conta e agent em `microbot_audit`.
- `internal/store/workers`:
  - sessão cifrada;
  - estado desejado;
  - token `ipl_` do worker, revogado ao trocar e ao parar;
  - `OtherClientSince`, a trava "alguém joga a conta em outro cliente".

  Toda escrita trava antes a linha da conta.
- `internal/agent`:
  - o `Link` virou um Hub de vários agents, roteado pela conta (seção 13.1);
  - o hello ganhou `account`, `kind`, `worker` e `bridge`;
  - `chronicle agent --worker`, que recebe a sessão pelo `worker_bootstrap`, grava os arquivos do
    HWID agent em memória (0600) e sobe o cliente com a sessão só no ambiente do processo, com o
    `flock` do cache;
  - no boot do worker:
    - liga o Chronicle Link e o FPS Control;
    - aplica `fpscontrol.limitFps=true` e `maxFps=20`, nomes conferidos no cliente 2.6.52 fixado.
  - SIGTERM: logout e `mb-client stop` em até 75 s, dentro dos 90 s de grace do pod.
- `internal/kube`: cliente REST mínimo (W1), o Deployment do worker e a verificação do token
  projetado. A verificação usa TokenReview, depois lê o pod, confere o uid e lê a conta da etiqueta
  do pod, nunca do que o agent diz.
- `internal/workerctl`:
  - o reconciliador: aplica os que rodam, apaga os parados e os órfãos, revoga o token;
  - o bootstrap no `OnConnect`, que não gera token novo quando o cliente do pod já roda;
  - `Restart`;
  - a visão dos pods, incluindo "não cabe": com quota, o ReplicaSet nem cria o pod (`ReplicaFailure`).
- API, sempre com `authed`, `microbotControl` e `?account=`:
  - `/api/microbot/*?account=`;
  - `GET /api/workers`;
  - `POST /api/workers/start|stop|restart`;
  - `GET|PUT|DELETE /api/accounts/session`.

  Tudo vai para a auditoria, sem o conteúdo da sessão.
- O bootstrap roda também com `WORKERS=off`, porque só precisa do banco e do link. Um worker subido à
  mão contra um servidor de dev (loopback) recebe a sessão do mesmo jeito.
- CLI:
  - `chronicle accounts connect --character <nome>`: lê o personagem no `accounts.json` do launcher e,
    das stores do HWID agent, só as linhas desse personagem;
  - `chronicle workers list|start|stop|restart`.
- Web:
  - Automação › Workers;
  - o seletor de conta no Cockpit e em Plugins, lembrado no navegador;
  - o VNC e o screenshot pela conta.

  Segue a estrutura da página 7 do canvas **sem** o painel de capacidade e sem o worker aberto à
  direita. A divergência é por decisão do dono: "tudo funcional primeiro, a aparência depois".
- Deploy:
  - `serviceAccountName: chronicle`;
  - `deploy/worker/Dockerfile` com `base.lock`;
  - `just worker-base-bump`;
  - o CI publica `chronicle-worker` e grava `WORKER_IMAGE` no overlay quando o `base.lock` existe.

**microbot-plugins:**

- `mb-client`: `MB_XMX`, `MB_HWID_STORE_DIR` e `MB_PREBUILT_PLUGINS`.
- Chronicle Link 1.11.0: `CHRONICLE_LINK_SERVER_URL` e `CHRONICLE_LINK_TOKEN` valem no lugar da
  config, e nada é gravado no perfil.
- A imagem `microbot-worker-base`: `worker/image.nix`, `worker/Dockerfile` e `worker/build.sh`.
- O `microbot-cli` fixado por `cliHash`.
- O primeiro CI (`gradle test` e a imagem na `main`).

**gielinor (`apps/chronicle`):**

- As SAs `chronicle` e `chronicle-worker`.
- O Role, mais o `auth-delegator`.
- A ValidatingAdmissionPolicy.
- A quota e a NetworkPolicy.
- O PVC `microbot-game-cache`.
- O README com a ativação.

### 14.2 Conferido

- Suíte Go inteira com Postgres 17, `golangci-lint` sem achados, `pnpm check`, `pnpm test` (95) e
  `pnpm lint`; no microbot-plugins, `gradle test`.
- **A política de admissão num kube-apiserver 1.32 real** (etcd e apiserver do `setup-envtest`, como
  processos): 43 de 43 casos. O worker do `WorkerDeployment` passa. Ficam de fora:
  - etiquetas faltando, réplicas a mais, `RollingUpdate`;
  - outra SA, outra imagem;
  - `hostPath`, segredo, outro PVC, `envFrom`;
  - privilégios;
  - mexer no Deployment do próprio app.
- **De ponta a ponta, sem cluster.** O kind não sobe aqui (cgroup v1). O teste juntou:
  - um `serve` em dev, com o cofre e o Postgres;
  - `accounts connect` com um `accounts.json` falso;
  - a imagem `microbot-worker-base` com o `chronicle` por cima, rodando `agent --worker` como 65532.

  O que aconteceu:
  - o agent ligou como `managed`, recebeu o bootstrap e subiu o cliente em 33 s;
  - o Chronicle Link do pod falou com o servidor usando o token do worker;
  - o FPS foi aplicado;
  - o Cockpit, o screenshot e o VNC ao vivo funcionaram pela conta;
  - nem a sessão nem o token ficaram no disco do pod;
  - a sessão gravada com outra chave foi recusada com a mensagem na lista, e um novo `connect`
    resolveu;
  - o restart do container refez o bootstrap e revogou o token anterior;
  - o SIGTERM saiu do jogo e parou o cliente.

### 14.3 Não conferido e pendente

- **A tela de login de verdade.** O sandbox recusa a porta 43594 da Jagex e intercepta o TLS. Falta
  conferir no saradomin, com uma conta logada e o dono olhando.
- **O reconciliador contra o k3s de verdade:** pods reais, o token projetado, a TokenReview, a quota e
  a NetworkPolicy. Só a admissão foi testada num apiserver.
- **O tour "Welcome to Microbot"** aparece a cada boot, porque a home do pod é nova. Fechar o tour
  automaticamente fica para a F5 (perfis).
- **`preferences.dat`** fica dentro do `jagexcache` compartilhado. São preferências do jogo e podem
  lembrar o nome do login antigo, que não é o da Jagex Account. Se incomodar, o PVC monta só os
  `main_file_cache.*` por `subPath`.
- **`Hello.Bridge` fica vazio:** a ponte do Chronicle Link ainda não tem rota de versão.
- A rota `/api/workers` não entrou no teste de contrato (`TestContract`).
- **Para ligar no saradomin, com o dono:**
  1. Fazer o merge do gielinor antes do chronicle: o app precisa da SA.
  2. Pôr o `CHRONICLE_VAULT_KEY` no SOPS.
  3. Criar as roles `microbot:control` e `microbot:agent` no Zitadel.
  4. Publicar a `microbot-worker-base` e rodar `just worker-base-bump`.
  5. Deixar os pacotes do GHCR públicos.
  6. `WORKERS=on` no overlay.
  7. `accounts connect` no zaros e o primeiro Iniciar.
