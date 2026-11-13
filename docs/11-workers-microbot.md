# 11 — Workers do Microbot: contas, perfis e clientes headless no saradomin

**Estado: rodada 1 implementada (2026-11-12) e ligada no saradomin, com o primeiro worker rodando
(seção 14.4); o que foi feito e conferido está na seção 14. Perfis (F5), rotinas no servidor (F6), métricas (F7) e GPU continuam como
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
| Imagem `chronicle-worker-base` | chronicle (`microbot/`, `nix/`, `deploy/worker/`) | Runtime do jogo pelo Nix, cliente fixado e nossos plugins |
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

A parte 1 da F5 (seção 15) implementou isto com algumas diferenças, anotadas lá: modelo de um nível
só, configurações da conta em linhas e não em jsonb, e mundo e gráfico para depois.

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

A F6 (seção 16) implementou isto com algumas diferenças, anotadas lá: só o tipo `trigger` por
enquanto, o run identificado pelo `uid` que o agent dá a ele, e a trilha com estado e texto. O tipo
`recurring` veio depois, na migração 0015 (seção 16.4); o `now` continua sendo o `run_start` do dono.

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
3. `PUT /api/accounts/{id}/session` com o bearer do login da CLI.

   Um personagem que o launcher guarda sem nome (`displayName` nulo, "Not set" na lista) se conecta
   pelo número na lista, com o nome do jogo: `--index 6 --as EventSourced`, como o `mb-account use
   --as` do harness. O servidor cifra e grava, e marca
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
4. O agent escreve na home em memória: `credentials.properties`, os arquivos de aparelho, e a config
   do Chronicle Link com a URL interna e o token.
5. `mb-client start --headless` com `MB_HEADLESS_GPU=1` e `MB_CLIENT_WRAPPER="vglrun -d egl"`.
6. Com o Agent Server de pé, o agent grava as configurações do perfil pelo `/settings/plugin`, instala
   os plugins do Hub do perfil e só então liga os `start_on_boot` (W7, seção 15).
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
- **Reiniciar** na mesma imagem grava a anotação `chronicle.ftm.dev.br/restarted-at` no template do
  pod, como o `kubectl rollout restart`: o Deployment (Recreate) para o pod e sobe outro. O servidor
  não apaga pods; a SA `chronicle` só lê pods (seção 7).
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
  list e watch em `pods`; get e list em `resourcequotas` e nos `pods` do `metrics.k8s.io` (F7,
  seção 18). Nenhuma permissão em secrets. `system:auth-delegator` para a TokenReview.
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
- **CPU e RAM por worker no painel:** o servidor lê o metrics-server (`metrics.k8s.io`, só os pods
  do namespace) e a quota (seção 18). A primeira versão previa o agent lendo o próprio cgroup; o
  metrics-server já roda no k3s, mede o mesmo que o `kubectl top` e não muda o agent nem o link.

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

Tudo sai deste repositório (decisão 35 do doc 01) e do mesmo CI (doc 07, seções 3.3 e 5):

- `chronicle-worker-base` pelo Nix (`dockerTools`, `nix/worker-runtime.nix`), do mesmo `flake.lock` do
  harness: JDK 21, Xvfb, x11vnc, os jars de `microbot/microbot.json`, o `microbot/bin` do harness e,
  por cima (`deploy/worker/base.Dockerfile`), os jars do `gradle -p microbot pluginJars`. Usuário
  65532. Quem constrói é o `deploy/worker/build-base.sh` (`just worker-base`).
- A tag da base é `<versão do Microbot>-<sha7 do último commit que mudou uma entrada da base>`. As
  entradas são `microbot/` (fora `*.md`), `nix/microbot.nix`, `nix/worker-runtime.nix`, `flake.nix`,
  `flake.lock`, `deploy/worker/base.Dockerfile` e `deploy/worker/build-base.sh`; `build-base.sh --tag`
  imprime a tag. O job `docker` (só na `main`) calcula a tag e só constrói e publica a base quando o
  GHCR ainda não a tem.
- `chronicle-worker:<sha>` (`deploy/worker/Dockerfile`): `FROM` essa base, mais o binário do mesmo
  commit. O job `docker` sempre a publica e grava `WORKER_IMAGE` no overlay no commit de deploy.
- Não há bump da base (W4): ela segue o commit. Um worker que já roda só troca de imagem quando o
  dono clica em Reiniciar (seção 6.5).
- O job `nix` roda os testes e os jars dos plugins em todo push e PR.

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
| `GET /api/workers` | Workers das contas do usuário: lugar, estado, versão, CPU e RAM, sessão, e quantos ainda cabem (seção 18) |
| `POST /api/workers/{conta}/start` \| `stop` \| `restart` | Ciclo de vida, auditado |
| `GET` \| `PUT /api/accounts/{id}/session` | Estado da sessão (sem o conteúdo) e conexão |
| `GET /api/client-profiles`, `GET` \| `PUT` \| `DELETE /api/client-profiles/{nome}` | Perfis do usuário: modelo, plugins e configurações; o `PUT` troca o perfil inteiro (seção 15) |
| `GET` \| `PUT /api/accounts/client?account=` | Perfil da conta, configurações próprias e o perfil efetivo; mundo e gráfico ficam para depois |
| `GET` \| `POST /api/microbot/triggers?account=`, `GET /api/microbot/runs?account=` | Gatilhos e histórico, no banco desde a F6 (seção 16); um agent anterior à F6 segue respondendo pelos arquivos dele |
| `GET /api/accounts/client/drift?account=`, `POST …/client/drift/save`, `POST …/client/apply` | O desvio do cliente em relação ao perfil, salvar os valores do cliente (na conta ou no perfil) e aplicar o perfil ao cliente (seção 17) |
| `/api/microbot/*?account=` | O que existe hoje, endereçado pela conta |

CLI: `chronicle accounts connect`, `chronicle workers list|start|stop|restart`,
`chronicle profiles list|show|put|delete|use|account`. Nenhuma abre o banco (regra 1 do CLAUDE.md).

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
| F0 | Link remoto: `MICROBOT_CONTROL=on` no saradomin, machine user do Zitadel para o agent do zaros, serviço no zaros (doc 08, fase 5) | o Cockpit do saradomin controla o cliente do zaros | gielinor, nix-config. O serviço do zaros está pronto no `nix-config` (PR #1 de lá), desligado até o machine user e o segredo existirem (doc 08, fase 5) |
| F1 | Hub por conta: hello com conta, kind, worker e a versão da ponte do Chronicle Link (o agent exige uma mínima e diz qual falta); rotas com `?account=`; mensagens `event` do agent; `microbot_audit.account_id`/`agent_id`; Automação › Workers com o zaros na lista | duas contas no mesmo servidor, cada uma com seu agent (o segundo pode ser um agent de teste no zaros) | chronicle |
| F2 | Contas e sessões: migração (`regular`, rota opcional, `managed`, `game_sessions`), cofre, `chronicle accounts connect`, Automação da conta na Conta | uma conta sem rota conectada pela CLI; a sessão aparece na web e nunca volta pela API | chronicle |
| F3 | Imagens e um worker aplicado à mão: as duas imagens e o CI delas, iGPU compartilhada, o PVC do cache, um Deployment escrito à mão que faz o bootstrap pelo link (sessão do cofre, `ipl_` do worker, perfil `minimal` fixo) | o dono entra no jogo pelo VNC do pod, o Chronicle Link dele alimenta o tracking; medir de novo com o perfil enxuto | chronicle (com `microbot/`), gielinor |
| F4 | Gerenciado: reconciliador, cliente REST do k8s (W1), RBAC e admission policy, Iniciar, Parar e Reiniciar pela web e pela CLI, trava da seção 6.2 | Iniciar e Parar sem `kubectl`; um pod apagado à mão volta para a tela de login | chronicle, gielinor |
| F5 | Perfis no Postgres: tabelas da seção 4.3, aplicar no boot, plugins do Hub do perfil, desvio e "Salvar no perfil", modelos | uma segunda conta sobe com o mesmo modelo e fica com os mesmos plugins e configurações | chronicle (com o export dos valores no Chronicle Link). **Parte 1 feita** (2026-11-12, seção 15): tabelas, modelos, aplicar no boot, plugins do Hub, API e CLI. **Parte 2 no servidor feita** (2026-11-13, seção 17): desvio, "Salvar no perfil", aplicar, e o perfil no cliente anexado. Falta a tela (fatia V8 da v7) |
| F6 | Rotinas no servidor: seção 4.4, gatilhos e histórico saindo dos arquivos, auditoria das ações do executor | o gatilho do gato sobrevive a um restart do pod; o histórico aparece no banco | chronicle. **Feita no servidor e no agent** (2026-11-13, seção 16); falta conferir num worker de verdade. As rotinas recorrentes vieram em seguida (seção 16.4), sem o loadout; a tela, na fatia V6 da web v7 |
| F7 | Capacidade e métricas: "cabe mais N", CPU e RAM por worker, aviso de sessão expirando, reinício na versão nova | por entrega | chronicle, gielinor. **Servidor feito** (2026-11-13, seção 18): "cabe mais N" pela quota, CPU e RAM por worker, login recusado na sessão. Falta a tela (resumo da Frota da v7) e decidir o reinício sozinho na versão nova |

Antes de cada fase com tela (F1, F2, F5, F6), o desenho entra no canvas e o dono aprova (regra 11).
Desde 2026-11-12 as telas destas fases são as da v7 (canvas, página 8; `docs/ui/decisoes-canvas.md`,
seção 7; decisão 36 do doc 01): a lista de workers é a Frota, os perfis da F5 aparecem em Plugins, as
rotinas da F6 em Rotinas, na Fila e no Histórico, e a capacidade da F7 no resumo da Frota. A página 7
fica como histórico.
Desde a parte 1 da F5, a configuração dos plugins que está no perfil volta a cada boot; o que foi
mudado direto no RuneLite pelo VNC e não entrou no perfil ainda se perde (é o desvio da parte 2).

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
| W4 | Bump da base manual ou automático | **Resolvida (2026-11-12):** sem bump. Com o repositório único (decisão 35 do doc 01), a base tem a tag das próprias entradas e segue o commit (seção 7.2) |
| W5 | Cache do jogo por pod ou compartilhado | **Resolvida (2026-11-12):** PVC compartilhado (seção 7.4) |
| W6 | Lista de workers antes dos gerenciados | **Resolvida** pela F1 |
| W7 | Como aplicar a config do perfil: escrever o arquivo de perfil do RuneLite antes de o cliente subir, ou `/settings/plugin` depois | **Resolvida (2026-11-12, dono):** pelo `/settings/plugin`, com o cliente de pé e **antes** de ligar os plugins, que assim leem a config certa no start. Não depende do formato dos arquivos de perfil; um ajuste que precise valer antes de o cliente abrir volta a pedir o arquivo |
| W8 | IP e escala de uma frota: proxy por worker, outro nó, outra casa | Adiar até haver mais de 3 contas; registrar o risco |
| W9 | Desvio de config: salvar sozinho ou pelo botão | Pelo botão ("Salvar no perfil"); a parte 2 da F5 seguiu esta recomendação (seção 17) |
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
- Recursos: `requests` 700m e 1 GiB; `limits` 1 núcleo e 1792 MiB; `MB_XMX=1024m` (o pedido de CPU
  subiu de 500m depois da medição da seção 14.4).
- Display: `MB_HEADLESS_SIZE=1048x503x24`, o tamanho exato do cliente: jogo em modo fixo (765×503)
  mais a barra lateral do RuneLite (283 px).
- Volumes: `home` (`emptyDir` em disco) em `/home/worker`; `secrets` (`emptyDir` em memória, 1 MiB)
  em `/run/chronicle` para os arquivos do HWID agent; o PVC `microbot-game-cache` em
  `/var/cache/chronicle/jagexcache` (`WORKER_GAME_CACHE`); o token projetado. O cache fica **fora**
  da home: montado em `.runelite/jagexcache`, o runtime criava `microbot-dev` e `.runelite` como root
  (755) e o cliente, uid 65532, não subia (visto no primeiro Iniciar no saradomin, 2026-11-12). O
  agent cria a `.runelite` e o link `jagexcache` → volume; o lock de boot fica no volume.
- A sessão da Jagex entra só como variável de ambiente do processo do cliente (`JX_SESSION_ID`,
  `JX_CHARACTER_ID`, `JX_DISPLAY_NAME`, como o launcher faz), nunca em arquivo.
- O agent segura `flock` em `jagexcache/.chronicle-boot.lock` do início do cliente até o Agent
  Server responder (seção 7.4).

### 13.4 Imagens

- `ghcr.io/felipemalacarne/chronicle-worker-base:<versão do Microbot>-<sha7 das entradas>`
  (`deploy/worker/build-base.sh`: `nix/worker-runtime.nix` mais os jars em `deploy/worker/base.Dockerfile`):
  - `/opt/microbot/bin`: o harness, no PATH;
  - o JDK 21, Xvfb, x11vnc, xdotool, bash, coreutils, util-linux, procps, jq, curl e fontes;
  - `MICROBOT_CLIENT_JAR` e `MICROBOT_HWID_AGENT_JAR` apontando para os jars fixados;
  - `MB_PREBUILT_PLUGINS=/opt/microbot/plugins` com os jars do `gradle pluginJars`;
  - usuário 65532, `HOME=/home/worker`, `MB_DEV_HOME=/home/worker/microbot-dev`.
- `mb-client` (`microbot/bin`) ganha as variáveis abaixo, e sem elas tudo continua como hoje:
  - `MB_XMX` (padrão `2g`);
  - `MB_HWID_STORE_DIR` (padrão `$HOME/.runelite`);
  - `MB_PREBUILT_PLUGINS`: instala os jars dali em vez de compilar o DevBootstrap e copia também o
    Chronicle Link;
  - pula o PipeWire e o `xdotool` quando não existem.
- `ghcr.io/felipemalacarne/chronicle-worker:<sha do chronicle>` (`deploy/worker/Dockerfile`): `FROM` a
  base com a tag das entradas no mesmo commit, com o `chronicle` em `/usr/local/bin/chronicle` e
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

  O `base.lock` e o `just worker-base-bump` saíram com o repositório único (seção 7.2).

**microbot-plugins** (hoje `microbot/`):

- `mb-client`: `MB_XMX`, `MB_HWID_STORE_DIR` e `MB_PREBUILT_PLUGINS`.
- Chronicle Link 1.11.0: `CHRONICLE_LINK_SERVER_URL` e `CHRONICLE_LINK_TOKEN` valem no lugar da
  config, e nada é gravado no perfil.
- A imagem `microbot-worker-base`: `worker/image.nix`, `worker/Dockerfile` e `worker/build.sh`. Hoje
  é a `chronicle-worker-base`: `nix/worker-runtime.nix`, `deploy/worker/base.Dockerfile` e
  `deploy/worker/build-base.sh`.
- O `microbot-cli` fixado por `cliHash`.
- O primeiro CI (`gradle test` e a imagem na `main`), hoje o job `nix` e o job `docker` do chronicle.

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

- **A tela de login de verdade.** O sandbox recusa a porta 43594 da Jagex e intercepta o TLS.
  Conferida no saradomin no mesmo dia (seção 14.4).
- **O reconciliador contra o k3s de verdade:** pods reais, o token projetado e a TokenReview foram
  conferidos no saradomin (seção 14.4). Falta ver o "não cabe" da quota e o que a NetworkPolicy recusa.
- **O tour "Welcome to Microbot"** aparecia a cada boot, porque a home do pod é nova. Resolvido na
  seção 15: o `mb-client --headless` passa `-Dmicrobot.tour=false`.
- **`preferences.dat`** fica dentro do `jagexcache` compartilhado. São preferências do jogo e podem
  lembrar o nome do login antigo, que não é o da Jagex Account. Se incomodar, o PVC monta só os
  `main_file_cache.*` por `subPath`.
- **`Hello.Bridge` fica vazio:** o hello sai antes de o cliente (e a ponte) subir no worker. Resolvido
  em 2026-11-13 pelo `status`: a ponte ganhou `GET /bridge/version` (Chronicle Link 1.12.0) e o
  agent lê a versão a cada `status` com o cliente rodando, no campo `bridge` (vazio sem ponte ou com
  plugin anterior; doc 06, adendo de 2026-11-13). Com o repositório único, o plugin e o agent do
  worker saem do mesmo commit; só o zaros pode ficar com um Chronicle Link mais velho que o agent, e
  agora isso aparece no `status`.
- A rota `/api/workers` entrou no teste de contrato (`TestContractPostgres`, 2026-11-12): um worker
  rodando, um parado por erro numa imagem velha e um sem sessão, mais um agent anexado.
- **Para ligar no saradomin, com o dono:**
  1. Fazer o merge do gielinor antes do chronicle: o app precisa da SA.
  2. Pôr o `CHRONICLE_VAULT_KEY` no SOPS.
  3. Criar as roles `microbot:control` e `microbot:agent` no Zitadel.
  4. Publicar a base do worker. Feito então com a `microbot-worker-base` e o `just worker-base-bump`;
     hoje o CI publica a `chronicle-worker-base` na `main` quando ela falta (seção 7.2).
  5. Deixar os pacotes do GHCR públicos.
  6. `WORKERS=on` no overlay.
  7. `accounts connect` no zaros e o primeiro Iniciar.

### 14.4 Primeiro worker no saradomin (2026-11-12)

Conta `acc_eventsourced`, conectada pelo `accounts connect --index 6 --as EventSourced`. O personagem
é "Not set" no launcher, e o zaros não tem arquivo de device id. O `random.dat` foi levado.

**O que funcionou de ponta a ponta:**

- Iniciar pela web e o pod criado pelo reconciliador.
- O bootstrap pelo token projetado, o cliente na tela de login e o login no mundo 566 pelo Cockpit.
- O VNC ao vivo pelo túnel e o Blackjacker iniciado pelo Cockpit.
- A auditoria por conta.

**O que o primeiro uso real achou e já foi corrigido:**

- PR #9: com o PVC montado em `.runelite/jagexcache`, o runtime criava `microbot-dev` e `.runelite`
  como root, e o cliente (uid 65532) não subia. O cache passou para `/var/cache/chronicle/jagexcache`,
  com o link criado pelo agent.
- PR #10: o fim de um run em dois passos deixava o run seguinte ser cancelado.
- Display de 1280×900:
  - a janela do cliente ficou com 1532×900 (painel do jogo de 1249 e barra lateral de 283) e cortava
    a barra lateral;
  - o jogo em modo fixo ocupava só 765×503 no meio do preto;
  - o display passou a 1048×503.
- O reconciliador reaplicava o Deployment a cada passada, então um template novo vindo de um deploy
  do servidor (recursos, variáveis, montagens) reiniciaria sozinho os workers em uso, deslogando a
  conta. Agora:
  - um worker que já roda a imagem certa não é tocado;
  - o template novo só chega por um Iniciar, pelo Reiniciar ou por uma imagem nova.

**Medido com o Blackjacker rodando** (render por software, `kubectl top`): **645m de CPU e 784 MiB**.
Parado na tela de login eram cerca de 0,43 núcleo e 800 MiB. A conclusão da seção 7.1 muda: com script
ativo, **o limite é a CPU, não a RAM**. Com cerca de 3,4 núcleos livres, cabem uns 4 workers ativos, e
a RAM daria para uns 8. O pedido subiu para 700m e a quota do namespace acompanha (gielinor). A GPU
(VirtualGL na iGPU) é o que mais baixaria esse número.

**Pendências deste uso:**

- **Barra lateral:** a do RuneLite fica visível no worker. A web ainda não tem tudo o que ela tem;
  trazer para o Cockpit, aos poucos, o que fizer falta.
- **Tour "Welcome to Microbot":** aparecia a cada boot por cima do jogo, porque a home do pod é nova.
  Resolvido na seção 15.
- **Configuração dos plugins:** a do Blackjacker e a dos outros se perdia quando o pod era recriado.
  Desde a seção 15, o que está no perfil da conta volta a cada boot.
- **FPS:** conferido em 2026-11-12 no worker da `acc_eventsourced` com o Blackjacker ativo:
  `fpscontrol` com `limitFps=true` e `maxFps=20` (`GET /api/microbot/config`), nenhum aviso do agent
  ao ligar o FPS Control nem ao gravar os valores, e 405m de CPU. Que o plugin está ligado só se vê
  assim, de forma indireta: a lista do Agent Server traz só os scripts do Microbot, e o agent recusa
  ler o grupo `runelite`.

## 15. F5, parte 1: perfis no servidor (2026-11-12)

Esta parte não tem tela. A web vem na parte 2: é a fatia V8 da web v7, Plugins da frota (artboard
`V7Plugins1440`, página 8 do canvas; `docs/ui/decisoes-canvas.md`, seção 7.5). Até lá, os perfis se
editam pela CLI e pela API. Um perfil nunca muda um cliente que já roda: vale
no próximo Iniciar ou Reiniciar.

### 15.1 Entregue

- **Migração 0013:** `client_profiles`, `client_profile_plugins`, `client_profile_settings`,
  `account_client` e `account_client_settings`.
- **Diferenças em relação à seção 4.3:**
  - o modelo tem um nível só: um modelo não tem modelo, e um perfil que já é modelo não ganha um;
  - as configurações próprias da conta são linhas (`account_client_settings`), não um jsonb, para
    passar pelas mesmas checagens;
  - plugin não tem a coluna `enabled`. Um plugin do perfil é instalado (os do Hub) e, com `start`,
    ligado no boot. Um plugin do Hub que liga precisa da classe (`class_name`): o Hub instala pelo
    nome interno, e o Agent Server liga pela classe;
  - mundo e gráfico ficam de fora. O mundo continua em `workers.world` (o Iniciar), e o gráfico
    espera a GPU (seção 7.3). Enquanto isso, o fixo do worker (Chronicle Link e FPS Control a 20) é
    o perfil de sistema, por baixo de tudo. Um perfil pode mudar o FPS, nunca desligar o Chronicle
    Link (classe protegida).
- **Validação:** as mesmas regras do `config_set` (grupos protegidos, chaves que parecem segredo,
  valor de até 2000 caracteres), no servidor ao gravar e no agent antes de aplicar.
- **`internal/agent`:**
  - `ClientProfile`, `ValidateProfile` e `MergeProfiles` (modelo, depois perfil, depois a conta);
  - o bootstrap leva o perfil efetivo;
  - o agent grava as configurações, instala os do Hub e liga os `start`, nessa ordem (W7);
  - o que o cliente recusa volta em `BootstrapResult.Problems` e na saída do `client_start` do
    Cockpit, e o resto do perfil é aplicado mesmo assim.
- **Reconciliador:**
  - manda o perfil efetivo no bootstrap;
  - o que o cliente recusou vira o `last_error` do worker (`profile: …`), visível na aba Workers e
    no `chronicle workers list`;
  - um agent que religa com o cliente de pé (acontece em todo deploy do servidor) não mexe mais no
    `last_error`.
- **API:** as rotas da seção 8, atrás de `authed` e `microbotControl`. As da conta usam
  `accountParam`. Em `AUTH_MODE=dev`, os perfis são do usuário `dev`.
- **CLI:** `chronicle profiles list|show|put|delete|use|account`. O `show` imprime o JSON que o
  `put -f` lê, então um perfil se edita como arquivo.
- **Tour:** o `mb-client --headless` passa `-Dmicrobot.tour=false`. No cliente 2.6.52 fixado, o tour
  só abre sem essa propriedade (`MicrobotTour.shouldAutoStart`). A mudança gera uma base nova do
  worker (seção 7.2).

### 15.2 Conferido

- Testes de tabela da validação e da junção.
- O boot do worker com um perfil:
  - a ordem é configurações, instalações e só então ligar;
  - um plugin do Hub cuja instalação falha não é ligado;
  - os problemas voltam no resultado;
  - o `client_start` seguinte reaplica o mesmo perfil;
  - um perfil inválido no bootstrap é recusado antes de tocar no Agent Server.
- O store no Postgres 17 (`TestClientProfilesPostgres`): um nível só, perfis por usuário, o perfil
  efetivo, e a recusa de apagar um modelo ou um perfil em uso.
- As rotas no Postgres (`TestClientProfileRoutesPostgres`) e em oidc (`TestClientProfileRoutesOIDC`):
  401 sem sessão, perfis do usuário da sessão e 404 para conta de outro usuário.
- A CLI contra uma API falsa e o reconciliador (perfil no bootstrap, problemas no `last_error`, o
  religar mantém o erro).
- `goose up/down/up` da 0013 e `just dbml`.

### 15.3 Não conferido e pendente

- **No jogo:** nada desta parte rodou ainda num worker de verdade. Conferir com o dono: um perfil
  com o Blackjacker e a configuração dele, Reiniciar o worker e ver a configuração voltar e o tour
  não aparecer.
- **Parte 2:** a tela (fatia V8 da web v7, Plugins da frota), o desvio (o Chronicle Link exportar
  os valores) e o "Salvar no perfil" (W9). O que o `V7Plugins1440` não mostrar disso vai para o
  canvas antes (regra 11). A versão da ponte no `status` saiu antes, em 2026-11-13 (seção 14.3).
- **Anexado (zaros):** o perfil ainda não se aplica; o zaros segue com a configuração no disco.

## 16. F6: rotinas no servidor (2026-11-13)

Os gatilhos e o histórico dos runs saem do disco do agent e vão para o Postgres. O executor continua
no agent (doc 08, R1). Sem tela nova: a web lê as mesmas rotas e os mesmos formatos de antes.

### 16.1 Entregue

- **Migração 0014:**
  - `routines`: por enquanto só o tipo `trigger`, um por evento e por conta, com o run que ele
    dispara em `spec`;
  - `routine_runs`: o run, identificado pelo `uid` que o agent dá a ele. O `run-N` recomeça num pod
    novo, então não serve de chave;
  - `run_events`: a trilha, uma linha por evento, com estado e texto.
- **Link, do agent para o servidor:** a mensagem nova `event` leva o run inteiro, com a trilha, a
  cada mudança.
  - Ao conectar, o agent manda o histórico de novo. Isso cobre o tempo em que o link esteve caído.
  - Uma fila em memória guarda o estado mais novo de cada run.
  - O servidor aplica cada run de forma idempotente: um snapshot com trilha menor que a gravada é
    mais velho e é ignorado, e um run nunca muda de conta.
  - O servidor valida o run antes de gravar (`agent.ValidateReport`), como faz com um pedido da web.
- **Link, do servidor para o agent:** o comando novo `routines_sync` manda, a cada conexão, os
  gatilhos da conta, que substituem os do agent.
  - É o que faz o gatilho do gato sobreviver a um restart do pod.
  - No agent anexado, o arquivo continua como cache, para o gatilho valer com o `serve` fora.
  - Se a conta ainda não tem gatilhos no banco, o primeiro agent anexado que conectar importa os do
    arquivo dele, uma vez só.
- **Compatibilidade:** um agent anterior à F6 não declara `routines_sync` no hello. O servidor não
  manda nada a ele, e as rotas de gatilhos e runs continuam falando com ele, como antes.
- **API:**
  - `POST /api/microbot/triggers` grava no banco e repassa ao agent. Com o agent fora, responde 200
    dizendo que ele recebe o gatilho quando ligar.
  - `GET /api/microbot/triggers` vem do agent (que sabe se está vigiando) e, com ele fora, do banco.
  - `GET /api/microbot/runs` vem do banco.
- **Auditoria:** o que o executor faz sozinho entra no `microbot_audit`, com o ator
  `trigger:<evento>` e a origem `agent`: o run que um gatilho começou (`run_start`) e como ele
  terminou (`run_end`). Os runs do dono continuam auditados onde começaram.
- **Pacotes:** `internal/store/routines` (o banco), `internal/routinesync` (a sincronização no
  `serve`) e, no `internal/agent`, o `event`, o `routines_sync`, o `uid` do run e a fila.

### 16.2 Conferido

- **No agent:**
  - o `routines_sync` troca os gatilhos e grava o arquivo, recusa uma lista inválida e nunca vem da
    web;
  - a fila guarda o estado mais novo de cada run;
  - a validação do run reportado;
  - cada mudança de um run sai com o mesmo `uid` e a trilha crescendo;
  - um histórico antigo ganha `uid`s estáveis;
  - de ponta a ponta pelo link, o histórico vai ao conectar e cada mudança vai na hora;
  - tudo com `-race`.
- **No store, Postgres 17:** gatilhos (gravar, substituir, importar só numa conta vazia) e runs
  (repetido, final, snapshot velho ignorado, conta errada recusada, conta desconhecida, run
  inválido, a leitura com a trilha).
- **No `routinesync`:** o agent antigo fica de fora; o worker recebe os gatilhos; o anexado importa
  uma vez; só os runs de gatilho são auditados.
- **Nas rotas, no Postgres, com a sincronização ligada como no `serve`:** o gatilho gravado com o
  agent fora chega a ele quando conecta; o histórico vem do banco; o agent antigo continua
  respondendo pelos arquivos dele.
- `goose up/down/up` da 0014 e `just dbml`.

### 16.3 Não conferido e pendente

- **Num worker de verdade:** ligar o gatilho do gato, Reiniciar o pod e vê-lo voltar.
- **O agent do zaros** só passa a usar isto com o `chronicle` atualizado lá (nix-config). Até lá ele
  segue com os arquivos, e a importação acontece na primeira conexão do binário novo.
- **Rotinas recorrentes:** feitas logo depois, na seção 16.4. A tela da v7 (Rotinas e a
  Fila da Visão geral) veio na fatia V6 (`docs/ui/decisoes-canvas.md`, seção 7.13).

### 16.4 Rotinas recorrentes (2026-11-13)

Entrega 4 da fase 4 do doc 08: o relógio do Time Tracking, ou um intervalo, dispara um run sozinho,
e várias recorrentes formam uma fila por prioridade. O comportamento do vigia, as travas e o que
ficou de fora (o loadout com banco) estão no Estado do doc 08. O break próximo entrou logo depois: a
rotina que vence espera o break do Break Handler (doc 08, "A troca").

- **Migração 0015**, na mesma tabela `routines`:
  - `kind` passa a aceitar `recurring`, e `event` fica nulo para ela;
  - colunas novas: `name` (único na conta), `clock` (a chave do `/bridge/timers`), `interval_min`
    (10 a 10 080) e `priority` (0 a 100);
  - uma `CHECK` garante a forma de cada tipo: o gatilho tem evento, a recorrente tem nome e relógio,
    intervalo ou os dois;
  - um índice por `(routine_id, started_at)` acha o último run de cada rotina.
- **Uma rotina recorrente**, no JSON (`agent.Recurring`):

  ```
  {"id": "rtn_…", "name": "Herb run", "enabled": true, "clock": "HERB", "intervalMin": 80,
   "priority": 10, "run": {"className": "…", "stop": ["…"], "timeoutS": 900},
   "updatedAt": "…", "lastRun": {"state": "done", "startedAt": "…", "finishedAt": "…"}}
  ```

  O `updatedAt` e o `lastRun` vêm do servidor e são ignorados ao salvar. O agent conta o intervalo
  a partir do `lastRun`, e as travas comparam um run que falhou com o `updatedAt`. Um run que a
  rotina começou leva o `routine` (o id), gravado em `routine_runs.routine_id`.
- **No agent:**
  - o `routines_sync` leva as recorrentes junto dos gatilhos, mas só para o agent que declara
    `recurring_get` no hello (um agent da F6 recusaria o campo desconhecido);
  - o servidor manda de novo a cada conexão e depois de cada `POST` ou `DELETE`;
  - `recurring_get` (leitura) devolve o que o vigia viu na última olhada: para cada rotina, se vence,
    o motivo (`clock_ready`, `clock_waiting`, `interval`, `held`, …) e quando vence; mais o que ele
    espera ("um run em andamento", "esperando: em combate") e o último run que começou;
  - o anexado guarda as recorrentes em `agent-recurring.json`; o worker, em memória.
- **API**, sempre com `?account=` (`accountParam`), atrás de `authed` e `microbotControl`:
  - `GET /api/microbot/recurring`: a visão do vigia. Sem agent, ou com um agent anterior a elas,
    vem do banco, com `watchError` dizendo por quê;
  - `POST /api/microbot/recurring`: cria (sem `id`) ou altera. Devolve `id` e `result` (a visão do
    agent, ou a do banco). Erros: 400 inválida ou mais de 20 por conta, 404 conta ou rotina de
    outra conta, 409 nome repetido;
  - `DELETE /api/microbot/recurring/{id}`: apaga; os runs dela ficam no histórico, sem a rotina.
- **Auditoria:** `recurring_set` e `recurring_delete` (origem `web`). O run que uma recorrente
  começa e como terminou entram com o ator `recurring:<id>` (origem `agent`), como os de gatilho.
- **Conferido:** os testes descritos no Estado do doc 08, a 0015 com `goose up/down/up` (com
  um gatilho e uma recorrente no banco) e o DBML regenerado.
- **Não conferido:** nada no jogo nem num worker de verdade. Também faltam a tela (desenhar no
  canvas antes) e a imagem nova do worker no saradomin.

## 17. F5, parte 2: desvio e "Salvar no perfil" (2026-11-13)

Esta parte é só servidor; a tela é a fatia V8 da web v7 (Plugins da frota). Ela também não muda o
protocolo do agent: o servidor usa os comandos que todo agent tem desde a fase 3 do doc 08
(`config_get`, `plugins`, `config_set`, `plugin_install`, `script_start`). Por isso funciona também
com o agent do zaros que ainda não foi atualizado.

### 17.1 Entregue

- **Desvio** (`GET /api/accounts/client/drift?account=`): o servidor lê do cliente o valor de cada
  configuração do perfil efetivo e a lista de plugins, e devolve:
  - cada configuração diferente do perfil, ou não definida no cliente;
  - cada plugin do Hub que não está instalado ou está em outra versão que a fixada;
  - cada plugin que devia estar ligado e não está;
  - em `unchecked`, os plugins do próprio RuneLite que ligam no boot: o `/scripts` do Agent Server
    lista só os do Microbot, então não dá para saber se estão ligados.
- **"Salvar no perfil"** (`POST /api/accounts/client/drift/save?account=`, W9: pelo botão, nunca
  sozinho): leva os valores atuais das chaves escolhidas para as configurações da conta
  (`"to":"account"`) ou para o perfil dela (`"to":"profile"`), que outras contas podem usar. Uma
  chave não definida no cliente é pulada e volta na resposta. Auditado como `profile_save`.
- **Aplicar / "Descartar"** (`POST /api/accounts/client/apply?account=`): devolve ao cliente os
  valores do perfil, na ordem do boot do worker (configurações, instalações do Hub, ligar o que
  não está ligado). Sem chaves, aplica o perfil inteiro; com chaves, só elas. Auditado como
  `profile_apply`.
- **Cliente anexado (zaros):** o "Iniciar" do Cockpit aplica o perfil da conta logo que o cliente
  sobe, se ela tiver um, e a resposta traz o que foi aplicado. Num worker, quem aplica é o agent
  dele, no boot (seção 15).

### 17.2 Conferido

- Testes de tabela do desvio: configuração diferente ou não definida, grupo não lido, plugin do Hub
  não instalado ou em outra versão, plugin parado, plugin do RuneLite não verificável. Também a
  leitura das listas do Agent Server e os lotes do `config_get`.
- As rotas no Postgres com um agent de verdade e um cliente falso que guarda o estado: desvio,
  salvar na conta e no perfil (com a chave não definida pulada), aplicar o perfil inteiro (o desvio
  zera depois), chave fora do perfil recusada, sem agent dá 503, e o "Iniciar" de um anexado
  aplicando o perfil.

### 17.3 Não conferido e pendente

- **Num cliente de verdade:** mudar uma opção pelo VNC, ver o desvio, salvar e descartar.
- **O que muda no RuneLite fora do perfil** não aparece: o desvio só olha as chaves que o perfil
  declara. Para ver tudo, o Chronicle Link teria de exportar todos os valores que não são segredo
  (seção 4.3), o que fica para quando a tela pedir.
- **A tela:** a fatia V8.

## 18. F7: capacidade e métricas (2026-11-13)

Só o servidor, a CLI e os tipos da web. A tela (o resumo da Frota da v7: "Cabe mais 1 worker no
saradomin", CPU e RAM, "Renovar sessão") é da fatia da Frota.

### 18.1 Entregue

- **`internal/kube`:** `ListPodMetrics` (`/apis/metrics.k8s.io/v1beta1/namespaces/<ns>/pods`),
  `ListResourceQuotas`, `ParseQuantity` (os sufixos da API, exato) e `WorkerResources`: os requests e
  limites do worker num lugar só, lidos pelo Deployment e pela conta da capacidade.
- **Reconciliador:** a cada passada (30 s) lê as métricas dos pods worker e as quotas do namespace.
  - **Uso:** CPU em núcleos (ao milinúcleo) e working set em bytes do pod que a visão mostra, não de
    um pod velho que ainda termina.
  - **Capacidade:** o menor, entre as quotas e os recursos que um worker ocupa, de piso(livre / o
    que um worker ocupa). `requests.cpu`/`cpu` e `requests.memory`/`memory` contam os requests do
    worker; `limits.*`, os limites; `pods`, `count/pods`, `count/deployments.apps` e
    `count/replicasets.apps`, um. Livre é `hard − used` do status, nunca negativo. Num empate, o
    limite é a CPU. Toda quota conta, mesmo com escopo: pode contar a menos, nunca a mais.
  - **Desconhecida, não zero:** sem quota que limite algo de um worker, ou com o status da quota
    ainda vazio, a capacidade fica ausente.
  - **Falha de leitura** (sem metrics-server, 403 sem a regra do Role): a passada segue e a parte some
    da resposta. O log avisa uma vez quando começa e uma quando volta.
- **`GET /api/workers`:** `usage` ao lado do `pod` (`cpu_cores`, `memory_bytes`, `measured_at`) e
  `capacity` no topo (`fits`, `limited_by`, `cpu_hard_cores`, `cpu_free_cores`,
  `memory_hard_bytes`, `memory_free_bytes`, `worker_cpu_cores`, `worker_memory_bytes`, `seen`).
  Tudo opcional: com `WORKERS=off` nada disso aparece.
- **Sessão expirando:** o servidor não vê quando a sessão da Jagex expira, então não há aviso por
  idade (a web tem o `pushed_at` para "enviada há 3 d"). O sinal é o login:
  - `login_failed` na sessão, derivado: o último login com ela não entrou. Um `accounts connect`
    novo ou um login que entra limpa;
  - o `last_login_result` existia desde a 0012, mas nada o gravava. Agora o login pelo Cockpit
    (`POST /api/microbot/login`) num worker gerenciado grava `ok`, ou o motivo do jogo quando o
    Agent Server responde 401 depois de esperar o login (credenciais recusadas, conta que não entra
    no mundo, login que não terminou em 60 s; `agent.LoginRefusal`);
  - login em andamento, o cooldown de tentativas e agent fora não gravam nada: não dizem nada da
    sessão. O anexado (zaros) usa a sessão do launcher, não a guardada, e também não grava.
- **CLI:** `chronicle workers list` mostra o uso, o login recusado e "Cabe mais N worker(s)".
- **Web:** só os tipos (`WorkerUsage`, `WorkerCapacity`, `usage`, `capacity`, `login_failed`) e o
  fixture do contrato.
- **gielinor (PR #16):** o Role `chronicle-workers` ganha get e list em `resourcequotas` e em
  `pods.metrics.k8s.io`, só no namespace.

### 18.2 Conferido

- O formato real, lido com `kubectl get` no saradomin em 2026-11-13: a quota `chronicle` (2200m,
  3328Mi e 4 Deployments; usados 720m, 1088Mi e 2) e as métricas do worker da `acc_eventsourced`
  (0,40 núcleo, 837 MiB). Os testes do `internal/kube` usam essas respostas; com elas, cabem mais 2.
- Testes de tabela da capacidade: sem quota, quota sem nada de worker, status vazio, acima da
  quota, `limits` e `pods`, duas quotas.
- O reconciliador com métricas 404, quota 403 e sem quota: a passada segue, cada aviso sai uma vez,
  e o uso é só o do pod atual.
- As rotas: uso e capacidade na lista, nada com `WORKERS=off`; o login gravado (401 grava, 409 não,
  anexado não). `login_failed` no Postgres 17. O contrato `workers` regenerado e o `pnpm check`.

### 18.3 Não conferido e pendente

- **No cluster:** depende do merge do gielinor #16 e de um deploy. Conferir no saradomin a
  `capacity.fits` da `/api/workers` (hoje daria 2) e o uso contra o `kubectl top pod -n chronicle`.
- **O que uma sessão da Jagex expirada dá no login** nunca foi visto. Se não for um 401 do Agent
  Server, o `login_failed` não acende; se for um login que só não termina, o motivo é "Login timed
  out after 60s".
- Logins por outros caminhos (troca de mundo, rotinas, a volta do jogo depois de uma queda) não
  gravam o resultado.
- **Reinício na versão nova:** o `outdated` e o Reiniciar já existem (seções 6.5 e 14.4). Reiniciar
  sozinho quando o worker está parado (o "Reiniciar na pausa" da Frota da v7) pede decisão do dono:
  o que conta como parado (deslogado, sem run, no break do Break Handler) e se a seção 7.2 ("só
  troca de imagem quando o dono clica") muda. Ficou de fora.
- **CPU e RAM do nó** (o "saradomin · CPU 3,1 / 4" do canvas) pediriam permissão de cluster em
  `nodes`. A web pode somar o uso dos workers e mostrar o teto da quota.
