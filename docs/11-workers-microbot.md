# 11 — Workers do Microbot no saradomin: arquitetura, imagens, builds e CI/CD

**Estado: proposta (2026-11-12). Nada deste doc está implementado.** Decidido com o dono em 2026-11-12:
workers headless no saradomin, um pod por conta, criados pelo próprio Chronicle; sessões das contas no
banco, cifradas, entregues ao worker pelo link do agent; tudo no namespace `chronicle`. O cliente do
zaros continua como hoje (doc 08), agora como um dos agents. W1 a W6 (seção 10) seguem abertas.
Protótipo da tela no canvas, página 7 ("Automação · workers": `V6MicrobotWorkers1440` e
`V6MicrobotWorkersEstados1440`), para o dono aprovar antes do código (regra 11 do CLAUDE.md). Está na
sequência do doc 04, seção 9, como item 19; registro no doc 01, decisão 34.

## 1. O que muda

Hoje o Chronicle controla **um** cliente: o `chronicle agent` roda na máquina do jogo, sobe o cliente
dev do harness (`mb-client start --headless`) e abre um link de saída para o `serve` (doc 08). O
`Link` do servidor aceita um agent só, e uma conexão nova derruba a anterior
(`internal/agent/link.go`).

A proposta:

- **vários agents ao mesmo tempo**, cada um com um id e uma conta do jogo;
- dois tipos de agent:
  - **anexado**: o do zaros, que o Chronicle controla mas não sobe nem derruba;
  - **gerenciado**: um pod no saradomin por conta, que o Chronicle cria no "Iniciar" e apaga no
    "Parar";
- as **sessões das contas** (a do launcher, mais os arquivos de aparelho que o `mb-client` já usa)
  guardadas no Postgres, cifradas;
- a tela Automação passa a ser uma **lista de workers**.

## 2. Medições que sustentam o desenho (saradomin, 2026-11-12)

Intel N95: 4 núcleos, 16 GB de RAM. Livre no momento: cerca de 3,4 núcleos e 7,7 GB; o host parado
fica em 18%. Medido com o cliente 2.6.52 preso a 2 núcleos (`taskset`), conta logada parada num
interior, FPS 20:

| Modo | CPU do cliente | RSS do java |
|---|---|---|
| GPU na iGPU (Xvfb + VirtualGL `-d egl`, Mesa 26.2.6, OpenGL 4.6) | 0,36 núcleo | 853 MiB |
| Software (o headless de hoje) | 0,43 núcleo | 808 MiB |

Na tela de login, com FPS 50: GPU 0,30 e software 0,20 núcleo. Nessa tela o VirtualGL custa mais
que o desenho, porque copia cada quadro da GPU de volta para o Xvfb.

Conclusões:

- um worker cabe com folga; o limite é a RAM, não a CPU, e dá para 3 a 4 workers ao lado do que já
  roda no cluster;
- a iGPU ganha pouco numa cena pequena; ainda não foi medida numa cena aberta ou cheia de jogadores;
- um script rodando e o `/observe` do cockpit somam CPU; isso também não foi medido.

Perfil gráfico do worker, escrito a cada boot:

- plugin GPU ligado, com draw distance 15–20, MSAA desligado, anisotrópico 0 e sem `unlockFps`;
- 117 HD desligado;
- FPS Control em 15–20;
- Entity Hider escondendo outros jogadores e os pets deles;
- Low Detail ligado;
- display de 1024×768 (o custo da cópia do VirtualGL cresce com a resolução).

## 2.1 Tela (canvas, página 7)

- **Workers** passa a ser a primeira aba da Automação. No topo, a capacidade do saradomin:
  - CPU e RAM pedidas no nó, separando os outros apps dos workers;
  - fatias da iGPU e quota do namespace;
  - quantos workers ainda cabem e o que limita.
- Embaixo, **uma linha por conta**: onde roda (zaros anexado ou pod com a versão), estado, CPU e RAM,
  sessão e ações. À direita, o worker aberto: CPU de 30 min, pod, imagem, recursos, perfil gráfico,
  Chronicle Link, agent e eventos.
- **Estados desenhados:**
  - Iniciar: onde e o mundo, com "entrar ao subir" desligado;
  - subindo, passo a passo;
  - Parar, com confirmação, porque sai do jogo;
  - conta em outro lugar: o Iniciar fica bloqueado;
  - sessão expirada, com o comando do push;
  - não cabe, dizendo quanto falta;
  - versão antiga: reiniciar desloga, então só no clique;
  - o seletor de worker no cockpit;
  - vazio.
- Fica de fora a versão de 640 px: gerenciar workers é coisa da tela cheia.

## 3. Componentes

```
                      ┌──────────────── namespace chronicle (saradomin) ───────────────┐
 navegador ──tailnet──► Ingress ─► chronicle (serve)                                    │
                      │            ├─ Hub de agents (link por id)                       │
                      │            ├─ cofre de sessões (AES-GCM, chave no Secret)       │
                      │            ├─ reconciliador de workers ──API k8s──┐             │
                      │            └─ Postgres (CNPG, ns databases)       │             │
                      │                  ▲ websocket (token da SA)        ▼             │
                      │            worker-<conta> (Deployment, 1 réplica)               │
                      │            └─ chronicle agent --worker                          │
                      │               ├─ Xvfb + x11vnc + VirtualGL ─► /dev/dri (iGPU)   │
                      │               └─ cliente Microbot (Chronicle Link, Agent Server)│
                      └────────────────────────────────────────────────────────────────┘
 zaros: chronicle agent (anexado) ──websocket (usuário de máquina do Zitadel)──► mesmo Hub
```

| Peça | Repositório | O que é |
|---|---|---|
| `chronicle serve` | chronicle | Ganha o Hub de agents, o cofre de sessões, o reconciliador e as rotas `/api/workers` |
| `chronicle agent` | chronicle | Dois modos: o de hoje (anexado, `MICROBOT_HARNESS_DIR`) e `--worker` (dentro do pod: recebe a sessão, escreve o perfil, sobe o cliente) |
| Imagem `microbot-worker-base` | microbot-plugins | O runtime do jogo, montado pelo Nix: o cliente fixado e os nossos plugins |
| Imagem `chronicle-worker` | chronicle | A base mais o binário `chronicle` do mesmo commit do servidor |
| RBAC, quota, GPU, rede | gielinor | O que o cluster precisa para o Chronicle criar workers com segurança |

## 4. Imagens

### 4.1 `microbot-worker-base` (repositório microbot-plugins)

Montada pelo Nix (`dockerTools`) a partir do mesmo `flake.lock` do harness. Assim, o JDK, o Mesa, o
libglvnd e o VirtualGL são exatamente os que foram testados no saradomin. Misturar nixpkgs diferentes
quebrou o java (glibc 2.42 contra 2.44) na medição de 2026-11-12.

Conteúdo:

- **runtime do jogo:**
  - JDK 21;
  - Xvfb, x11vnc e xdotool;
  - VirtualGL 3.0.2, Mesa e libglvnd, com `__EGL_VENDOR_LIBRARY_DIRS` e `LIBGL_DRIVERS_PATH` já
    definidos;
  - bash, coreutils, jq e curl, de que os scripts dependem;
- **o cliente:** os jars fixados em `microbot.json` (cliente e HWID agent);
- **os nossos plugins:** `gradle pluginJars`, incluindo Chronicle Link e DevBootstrap;
- **o harness:** `bin/mb-client`, `bin/lib.sh` e `bin/mb-account`, com as opções `MB_CLIENT_WRAPPER`
  e `MB_HEADLESS_GPU`;
- **o modelo do perfil gráfico** da seção 2.

Regras da imagem:

- usuário numérico (65532), sem root;
- tag `<versão do Microbot>-<sha7 do microbot-plugins>`;
- pacote público no GHCR, como o `chronicle`; o repositório continua privado;
- tamanho esperado de 0,7 a 1 GB; o Mesa traz o LLVM do llvmpipe.

### 4.2 `chronicle-worker` (repositório chronicle)

`deploy/worker/Dockerfile`:

```dockerfile
FROM ghcr.io/felipemalacarne/microbot-worker-base@sha256:<digest>   # fixado em deploy/worker/base.lock
COPY --from=go /out/chronicle /chronicle
USER 65532:65532
ENTRYPOINT ["/chronicle", "agent", "--worker"]
```

O estágio `go` é o mesmo do `Dockerfile` do servidor. A tag é o sha do commit, a mesma do
`chronicle`, então o agent do worker e o servidor saem sempre do mesmo commit.

## 5. Builds e CI/CD

### 5.1 microbot-plugins (workflow novo; hoje não há CI)

1. `gradle test`, a suíte de 115 testes, a cada push e PR.
2. Na `main`, quando mudarem `microbot.json`, `src/`, `bin/` ou o flake:
   `nix build .#worker-base-image`, push para `ghcr.io/felipemalacarne/microbot-worker-base:<tag>`
   e o digest no resumo do job. O cache do Nix fica no Actions (magic-nix-cache).
3. A atualização da base no chronicle é manual para começar (W4): `just worker-base-bump <tag>` grava
   o digest em `deploy/worker/base.lock`, e o commit segue o fluxo normal.

Localmente: `nix build .#worker-base-image && docker load < result` para testar a imagem. O harness
do zaros continua igual.

### 5.2 chronicle (amplia o job `docker`)

1. Constrói `chronicle` e `chronicle-worker` no mesmo commit e publica os dois no GHCR.
2. O commit de bump (`deploy: saradomin roda <sha7>`) grava as duas tags:
   - `newTag` do `chronicle` no overlay;
   - `WORKER_IMAGE=ghcr.io/felipemalacarne/chronicle-worker:<sha>` no configMap do overlay; é por
     ele que o reconciliador sabe qual imagem usar.
3. O Argo aplica o servidor novo. Os workers que estão rodando continuam na imagem antiga até o dono
   reiniciá-los (seção 6.5).

### 5.3 gielinor (uma vez, mais as mudanças da seção 7)

Por commit, e o merge na `main` é o deploy, como sempre.

## 6. Como funciona

### 6.1 Cadastrar a sessão de uma conta

No zaros:

```
chronicle accounts session push --account acc_x --character EventSourced
```

O comando lê só a entrada desse personagem do `~/.microbot/accounts.json` e os arquivos de aparelho
da conta, e manda tudo para `PUT /api/accounts/{id}/session` com o bearer do login da CLI.

O servidor cifra o pacote com `CHRONICLE_VAULT_KEY` (AES-256-GCM, com id de chave para rotação) e
grava em `game_sessions`. A API nunca devolve a sessão. O painel mostra o personagem, a data do push
e o resultado do último login. Quando a sessão expira, o painel diz o que fazer: abrir o launcher no
zaros e rodar o push de novo. Reimplementar o login da Jagex fica de fora.

### 6.2 Iniciar

O dono clica em Iniciar na linha da conta, e o pedido vai para `POST /api/workers/{conta}/start`.

1. **Trava.** O servidor recusa se a conta já está ativa em outro agent: o anexado do zaros informa a
   conta no hello e no estado, e um worker gerenciado tem a sua linha em `workers`. Uma conta fica
   num lugar só.
2. **Estado desejado.** `desired_state = running` na tabela `workers`.
3. **Deployment.** O reconciliador cria `worker-<conta>`:
   - etiquetas `app.kubernetes.io/component: worker` e `chronicle.ftm.dev.br/account: <conta>`;
   - imagem `WORKER_IMAGE`;
   - recursos: `requests` 500m de CPU e 1 GiB, `limits` 1 núcleo e 1,5 GiB, mais
     `gpu.intel.com/i915: 1` (compartilhado, seção 7);
   - ServiceAccount `chronicle-worker`, com um token projetado de audiência `chronicle`;
   - volumes `emptyDir` em memória para a home (sessão, perfil) e `emptyDir` em disco para o cache do
     jogo;
   - `CHRONICLE_URL=http://chronicle.chronicle.svc:8080`, ou seja, tráfego interno ao cluster, sem
     passar pelo Ingress.

### 6.3 Boot do worker

1. O `chronicle agent --worker` conecta ao Hub com o token da ServiceAccount.
2. O servidor valida o token pela TokenReview do k8s e reconhece o pod pelo nome que vem no token.
3. O hello leva o id do worker, a conta e a versão.
4. O servidor manda pelo link:
   - a sessão decifrada;
   - o token `ipl_` da conta para o Chronicle Link, criado como os de hoje;
   - as opções do perfil.
5. O agent escreve na home em memória:
   - `credentials.properties`, como o `mb-account use` faz hoje;
   - os arquivos de aparelho;
   - o perfil gráfico;
   - a config do Chronicle Link: URL interna e Credential Token.
6. O agent roda `mb-client start --headless` com `MB_HEADLESS_GPU=1` e
   `MB_CLIENT_WRAPPER="vglrun -d egl"`.
7. O cliente fica na tela de login. Entrar é outro clique (Entrar, com o mundo), salvo a opção da W3.

### 6.4 Uso

É o que o doc 08 já faz (cockpit, VNC pelo túnel do servidor, rotinas e triggers), com uma diferença:
cada chamada leva o id do agent. O Chronicle Link do worker envia o estado da conta para o servidor
pela URL interna. A auditoria (`microbot_audit`) ganha a coluna `agent_id`.

### 6.5 Parar, reiniciar, atualizar e falhas

- **Parar:** `desired_state = stopped`. O agent sai do jogo (logout limpo, se estiver logado) e para o
  cliente, e o reconciliador apaga o Deployment. A sessão some junto com a memória do pod.
- **Atualizar:** depois de um deploy, os workers antigos aparecem como "versão antiga" na lista.
  Reiniciar desloga a conta, então só acontece no clique do dono ("Reiniciar na versão nova") ou
  sozinho quando o worker está parado. O protocolo do link continua aditivo, então um agent antigo
  segue funcionando com o servidor novo.
- **Falhas:**
  - **o pod cai:** o Deployment o recria; o agent reconecta e o cliente volta para a tela de login,
    sem entrar sozinho;
  - **o link cai:** o agent segue o run (decisão R1 do doc 08) e enfileira a auditoria;
  - **o servidor reinicia:** os workers reconectam, e o reconciliador compara o estado desejado com
    os Deployments que existem.

## 7. Cluster (gielinor)

- **Namespace:** `chronicle`, o que já existe, para o web, os workers e tudo do Chronicle. O Argo não
  apaga os Deployments dos workers: ele só remove o que criou, e reconhece isso por uma etiqueta
  própria que os workers não têm.
- **ServiceAccount `chronicle`** (o servidor):
  - um Role no namespace com create, delete, get, list, watch e patch em `deployments`, e get, list e
    watch em `pods`;
  - **sem permissão em secrets**;
  - um ClusterRoleBinding para `system:auth-delegator` (TokenReview).
- **ValidatingAdmissionPolicy** (GA no k8s 1.35, a versão do saradomin): pedidos da SA `chronicle`
  em Deployments só passam com:
  - a etiqueta `component: worker`;
  - a SA `chronicle-worker`;
  - uma imagem `ghcr.io/felipemalacarne/chronicle-worker`.

  Assim o servidor não consegue alterar o próprio Deployment nem criar outra coisa.
- **ServiceAccount `chronicle-worker`:** sem permissão nenhuma e com
  `automountServiceAccountToken: false`. Só recebe o token projetado de audiência `chronicle`,
  renovado pelo kubelet; o agent relê o arquivo a cada reconexão.
- **ResourceQuota:** um teto de `requests` no namespace, somando o web e os workers. Um worker a mais
  do que cabe fica Pending, em vez de apertar o resto do nó, e o painel mostra isso.
- **iGPU compartilhada:** hoje o `intel-gpu-plugin` (`infrastructure/intel-gpu`) roda sem
  `-shared-dev-num`, então há um único slot `gpu.intel.com/i915`, e ele está com o Jellyfin. Com
  `-shared-dev-num=4`, o Jellyfin e até três workers dividem o `renderD128`. A divisão é por fatia
  de tempo, sem isolamento, o que basta para isso.
- **NetworkPolicy:** os workers só saem para o Service do chronicle e para a internet (servidores do
  jogo). Os outros serviços do cluster, como os bancos, ficam fora de alcance.
- **Segredo:** `CHRONICLE_VAULT_KEY` no `chronicle-secrets`, via SOPS.

## 8. Mudanças por repositório

**chronicle**

- `internal/agent`:
  - o `Link` vira um Hub de agents por id;
  - o hello ganha `worker`, `account` e `kind`;
  - o modo `--worker`: bootstrap da sessão, escrita do perfil, desligamento limpo;
  - as rotas de hoje, de agent único, continuam enquanto a web migra.
- **Migração nova:**
  - `workers`: id, conta, tipo, estado desejado e observado, imagem, datas;
  - `game_sessions`: conta, personagem, texto cifrado, nonce, id da chave, datas;
  - uma conta ativa por vez;
  - `microbot_audit.agent_id`.
- **Pacotes novos:**
  - `internal/store/workers` (repositório e cofre);
  - um cliente mínimo da API do k8s, só Deployments e TokenReview (W1);
  - o reconciliador no `serve`, ligado por `WORKERS=on`.
- **API e CLI:**
  - `/api/workers`: listar, iniciar, parar, reiniciar;
  - `/api/accounts/{id}/session`: estado e push;
  - `chronicle accounts session push`.
- **Web:** a lista de workers na Automação, com estado, onde roda, CPU e RAM, sessão e as ações. O
  artboard vem antes no canvas (regra 11).
- **Deploy:** `deploy/worker/` (Dockerfile e `base.lock`), o job `docker` publicando as duas imagens e
  `WORKER_IMAGE` no overlay.

**microbot-plugins**

- `bin/mb-client`: `MB_CLIENT_WRAPPER` e `MB_HEADLESS_GPU`, já testados no saradomin e ainda sem
  commit.
- No flake: `packages.worker-base-image` e o modelo do perfil gráfico.
- `.github/workflows/ci.yml` (seção 5.1).

**gielinor**

- O RBAC, a ValidatingAdmissionPolicy, a quota, a NetworkPolicy e as duas ServiceAccounts.
- `-shared-dev-num` no `intel-gpu-plugin`.
- `CHRONICLE_VAULT_KEY`.
- `MICROBOT_CONTROL=on` no overlay e o usuário de máquina do Zitadel para o agent do zaros (doc 08).

## 9. Fases

| Fase | Entrega | Onde |
|---|---|---|
| F1 | Hub com vários agents, auditoria por agent, lista de workers na Automação com o zaros (anexado) como primeiro item; `MICROBOT_CONTROL=on` no saradomin com o agent do zaros | chronicle, gielinor |
| F2 | As duas imagens e os dois CIs; GPU compartilhada; um worker de teste aplicado à mão com a sessão num Secret SOPS provisório; medir de novo com o perfil enxuto | microbot-plugins, chronicle, gielinor |
| F3 | Cofre de sessões, `session push`, reconciliador, RBAC e admission policy, Iniciar e Parar; o Secret provisório sai | chronicle, gielinor |
| F4 | Capacidade ("cabe mais N"), CPU e RAM por worker no painel, aviso de sessão expirando, reinício na versão nova | chronicle |

## 10. Decisões abertas

| # | Decisão | Recomendação |
|---|---|---|
| W1 | `client-go` ou um cliente REST mínimo para a API do k8s | REST mínimo: só Deployments e TokenReview, sem ~20 MB de dependências no binário |
| W2 | Base pelo Nix (`dockerTools`) ou Dockerfile Debian | Nix: o mesmo pin do harness, a combinação testada |
| W3 | O worker entra no jogo sozinho ao subir? | Não: Entrar é um clique separado; no máximo uma opção por worker, desligada por padrão |
| W4 | Atualização da base no chronicle: manual ou automática entre repositórios | Manual (`just worker-base-bump`); automatizar depois com um token de escopo fino |
| W5 | Cache do jogo por pod (`emptyDir`) ou compartilhado (PVC) | Por pod primeiro; compartilhar se o boot ficar lento |
| W6 | Ligar a lista de workers antes da F3, só com o zaros | Sim: a F1 já entrega a lista, e o worker gerenciado entra nela depois |

Riscos de engenharia:

- várias contas na mesma máquina, no mesmo IP e com o mesmo dono ficam ligadas entre si: uma ação da
  Jagex contra uma tende a pegar as outras;
- a GPU dividida não tem isolamento: um worker travado na GPU afeta o Jellyfin e os outros workers.
