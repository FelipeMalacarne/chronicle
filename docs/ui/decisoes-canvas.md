# Decisões de UI do canvas (Claude Design)

Registro das decisões tomadas na reestruturação das telas do planner no canvas do Claude Design
(<https://claude.ai/artifact/NqmKesmqCJH1gnSowjJFbQ>), com o design system **Pergaminho**
(<https://claude.ai/artifact/WrVkgXPtCrRJPeUSgpHWVC>). Complementa `brief-ui-ux.md` (brief) e segue o
modelo de domínio de `../09-modelo-de-dominio.md`. Nada aqui altera código em `web/` nem os docs
numerados.

Histórico: as seções 1 a 3 foram escritas para a página v4 (2026-11-04). Em 2026-11-05 o canvas foi
consolidado numa única página, **v6**, e as páginas v0 a v5 foram removidas (seção 5). Onde a v6
revisou uma decisão da v4, a seção 5 prevalece.
Em 2026-11-12 o dono pediu a reestruturação
para várias contas; ela está na página **8 · v7 · Frota** (seção 7), que prevalece sobre a v6 nas telas
que redesenha. As páginas v6 ficam no canvas como histórico e como referência das telas que a v7 ainda
não redesenhou.

## 1. Decisões

1. **Sem tela de comparação.** O brief (seções 0, 6 e 7) previa "Comparar rotas". Decisão do dono: cada
   rota é só mais uma rota entre as N possíveis; o foco é o domínio (conta, objetivos, entidades). A
   referência cruzada aparece em três lugares discretos: no painel da entidade em Mundo ("rotas que
   tocam esta entidade"), numa linha discreta do passo ("também em BRUHsailer CH1.6") e na cobertura em
   percentual na lista de Rotas.
2. **Navegação A: Agora · Rotas · Mundo · Conta.** Microbot só entra na sidebar quando
   `MICROBOT_CONTROL` está ligado (doc 08). Metas vivem dentro de Mundo e nos painéis do Agora, não
   como item de navegação.
3. **Direção visual Pergaminho.** Linguagem da interface do OSRS sem copiar assets: papel do quest
   journal de dia (tema `pergaminho`), pedra e madeira à noite (tema `pedra`), bevel de 1px, placas de
   título em dourado sobre madeira, tricolor de quest (verde / amarelo / vermelho). Alegreya para
   display, Inter para interface, JetBrains Mono para dados. Toggle de tema em toda tela.
4. **O passo não é uma linha.** Era a única reclamação sobre a UI anterior. O passo atual é um card com
   anatomia completa: texto do guia, local, chips de objetivo com papel (conclui / inicia / requer /
   usa) e veredito, itens do trecho de banco, evidência com fonte e hora, cross-ref discreta e ações
   (feito, pular, fixar outro).
5. **Passo de parágrafo (BRUHsailer) = TL;DR + bullets recolhíveis.** O TL;DR do guia fica aberto, o
   passo a passo recolhido; metadados (GP, tempo) viram chips; objetivos mostram progresso parcial
   ("1 de 4 quests").
6. **Veredito `unknown` nunca é `unmet`.** Chips e linhas distinguem "não sabemos" (pontilhado, "?") de
   "não atende" (vermelho). O estado `implied` (presumido pela cascata) só existe na rota ativa, mostra
   "confirmar" e nunca vira `auto_done`.
7. **Banco em três colunas: com você / no banco / o guia manda.** Ações de banco são instrução, nunca
   passo concluído. Frescor (`live`, `restored`, `unavailable`) aparece como badge; ausência só é
   afirmada com container `live` e baseline `full`.
8. **Ícones reais do jogo.** Skills, itens e quest point usam sprites do OSRS Wiki (cacheados
   localmente no produto), com rodapé de atribuição "© Jagex, via Old School RuneScape Wiki,
   CC BY-NC-SA 3.0". Atribuição dos guias (OzirisLoL / ironman.guide; So Iron BRUH e ParasailerOSRS)
   em toda tela com passos.
9. **Grafo de dependências mantido** como explorador em Mundo (V4Grafo e versão celular), com nó em foco,
   requisitos e desbloqueios e filtros por tipo de entidade.
10. **Telas de controle do Microbot** (visão geral, rotinas, plugins e celular) desenhadas conforme o
    doc 08: desligado por padrão, lista fechada de comandos, VNC só quando habilitado, auditoria de
    `run_event`.
11. **Recorrentes como entidades com prazo** (proposta ao doc 09, ainda não aprovada): birdhouses, herb
    run, kingdom aparecem em Mundo com `ready_at`, o Agora lista "pronto em 38 min" e a rotina do
    Microbot pode ser disparada "pelo recorrente" além do cron. Marcado como proposta nos artboards.
12. **Onboarding e sessões.** Fluxo de primeiro acesso em seis passos (RSN e modo, camadas do guia,
    rota inicial, import do JSON do ironman.guide como `manual_done · causa import`, plugin, primeiro
    sync) e tela de sessão com diff "o que mudou / o que isso causou", derivado dos eventos de
    `objective_progress` e `step_status` entre snapshots.

## 2. Artboards da página v4 (histórico; hoje em v6, seção 5)

| Artboard | Conteúdo |
|---|---|
| V4Fundamentos | tokens, átomos (célula de skill, slot, linha de passo, chip de objetivo, placa) nos dois temas |
| V4AgoraOziris | Agora com rota Oziris ativa (1.1.73–1.1.85, trecho de banco 1.1.70–1.1.113); interativo: feito, pular, desfazer, ⌘K, fixar outro |
| V4AgoraBruh | Agora com BRUHsailer ativa (CH1.6 com TL;DR + passo a passo recolhível) |
| V4Agora1024 | Agora a 1024px: sem sidebar, barra no topo, painéis laterais abaixo; mostra a faixa "hiscores indisponível" |
| V4Rotas, V4RotaOziris, V4RotaBruh | lista de rotas com cobertura e as duas rotas abertas |
| V4Mundo, V4Grafo, V4Meta | entidades, grafo de dependências e meta Barrows gloves |
| V4Conta, V4Sessao, V4Setup | conta e fontes, diff de sessão, primeiro acesso |
| V4Vazios | estados vazios e de erro (sem plugin, fonte falhou, passo sem objetivo, conflito) |
| V4Microbot, V4MicrobotRotinas, V4MicrobotPlugins | controle do cliente (doc 08) |
| V4Mobile* | Agora Oziris e BRUHsailer, rota, grafo e Microbot a 390px |

## 3. O que é mock

- Conta "Iron Felipe", níveis, horários, sessões e o percentual das metas são dados de exemplo.
- Os passos, locais, itens e cross-refs vêm dos `guide.json` reais (Oziris e BRUHsailer); os objetivos
  dos passos BRUHsailer são ilustrativos até a extração por agente (doc 09, seção 6).
- Recorrentes com `ready_at` e o gatilho "pelo recorrente" são proposta, não contrato.
- Quests dependem da faceta do plugin 1.1; nos artboards aparecem como "não sabemos" onde ainda não há
  fonte.

## 4. Design system Pergaminho

Tokens em dois temas, brand book e, desde esta revisão, componentes React reais (`Theme`, `Plaque` /
`Panel`, `SkillCell`, `InventorySlot`, `StepRow`, `ObjectiveChip`, `BankCard`) com `bundle.css`,
`bundle.js`, `index.d.ts`, README e preview por componente. No SPA Svelte os mesmos nomes de classe
(`pg-*`) e os tokens valem sem React; a implementação em `web/` fica para a sequência do doc 04,
seção 9.

## 5. Consolidação na v6 (2026-11-05)

Decisões da v6, tomadas com o dono numa sessão paralela e registradas nas notas do canvas:

1. **Nome Chronicle, no produto** (decidido em 2026-11-05, decisão 30 do doc 01). Wordmark Alegreya
   700 com o símbolo do livro e a runa, em ouro sobre madeira; "Ironman Planner" vira descrição. Entra
   no título da SPA, na sidebar, no primeiro acesso e no README na implementação.
2. **Limpeza sem esconder.** A v5-limpeza falhou por esconder demais (menus, banco recolhido). Na v6
   nada some: cada fato continua na tela, com menos camadas. Explicação nunca em parágrafo: vai para
   o `title` ou some.
3. **Um bisel só, o do painel.** Dentro dos painéis tudo é plano (células, chips, slots e linhas
   separados por fio); seleção é faixa dourada com barra à esquerda. Sidebar opção A (madeira
   inteira, refinada) em todas as telas de 1440 px.
4. **Dois alvos: 640 px** (janela ao lado do jogo) **e 1440 px** (olhar analítico), mesmos componentes.
5. **A lista de Rotas sai.** "Rotas" abre a rota ativa na seção do cursor, com abas Oziris ·
   BRUHsailer · Comparar. A aba da rota inativa mostra onde você estaria e "Tornar ativa".
6. **Comparar é aba de Rotas** (decidido em 2026-11-05, decisão 29 do doc 01), revisando a decisão 1
   da v4 a pedido do dono. Não é a tela principal.
7. **Mundo**: lista "no seu caminho agora" e "feitas há pouco"; o painel de entidade é o mesmo em todas
   as telas e abre por cima, sem sair da página.
8. **Conta**: skills e quests com fonte e idade primeiro, depois containers com frescor, fontes no fim.

Portado das páginas antigas para a v6 na consolidação: Agora e Rota com a BRUHsailer (1440 e 640),
Comparar, Agora com fonte falhou / Fixar outro / recorrentes, estados sem dados, sessão, primeiro
acesso e Automação (Microbot: cockpit, rotinas, plugins, 640). Os artboards portados usam a casca da
v6 e, desde 2026-11-05, também o registro da v6 (sem texto de sistema, explicações no `title`, um bisel
só). A página v6 é a referência para a implementação, tela por tela, começando pelo Agora. Em aberto:
recorrentes com `ready_at` como entidade (decisão 31 do doc 01).

## 6. Implementação da v6 (2026-11-05)

As telas da página v6 foram implementadas em `web/` (componentes em `web/src/lib/v6`, folha em
`web/src/lib/styles/pergaminho.css` gerada a partir das regras dos artboards V6, com os mesmos nomes de
classe). Artboards seguidos:

| Tela | Artboards | Rota |
|---|---|---|
| Casca (Chronicle, sidebar A, barra estreita) | V6Agora1440, V6Agora640, V6Sidebars (opção A) | todas |
| Agora | V6Agora1440, V6Agora640, V6AgoraBruh1440, V6AgoraBruh640, V6AgoraEstados1440 (fonte falhou, Fixar outro; o painel Recorrentes implementado em 2026-11-10 com os relógios do Time Tracking: a placa diz "Time Tracking · há 3 min" no lugar de "proposta", sem o sufixo "· Microbot"/"· manual", e a 640 o painel vem antes do rodapé); Recorrentes e Aqui perto com os estados, V6AgoraPerto1440 (página 1c, 2026-11-10, implementada direto e sincronizada no canvas) | `/` |
| Painel de entidade | V6Entidade, comportamento em V6Agora1440/640 e V6Mundo1440 | sobre qualquer tela |
| Rota | V6Rota1440, V6Rota640, V6RotaBruh1440 | `/rotas/<guia>/<seção>` |
| Comparar | V6Comparar1440 | `/compare` |
| Mundo › Entidades | V6Mundo1440, V6Mundo640 | `/mundo` |
| Mundo › Metas | V6Metas1440, V6Metas640 | `/mundo/metas` |
| Mundo › Grafo (2026-11-08) | V6Grafo1440 (colunas), V6Grafo640 (árvore) | `/mundo/grafo?focus=` |
| Agora · passo-parágrafo por dentro (2026-11-08, implementado direto) | V6PassoAcoes640, página "1b · Agora · passo-parágrafo por dentro" | `/` |
| Conta | V6Conta1440, V6Conta640; combat achievements e collection log, V6ContaConquistas1440 (página 4b, 2026-11-10, implementada direto e sincronizada no canvas) | `/conta` |
| Primeiro acesso | V6Setup1440 | `/setup` |
| Automação › Cockpit (2026-11-08, doc 08 fases 1 e 2) | V6Microbot1440 (só dashboard e VNC ao vivo), V6Microbot640 (só dashboard); com a conta logada, V6MicrobotCliente1440, V6MicrobotClienteEstados1440 e V6MicrobotCliente640 (página 6, aprovada em 2026-11-09; substitui a 5b; entregas 1 e 2 feitas: os estados marcados "ponte" valem com o plugin Chronicle Link 1.4 no cliente de dev); Use e Cast com alvo e o mapa do mundo, V6MicrobotUsarEm1440 e V6MicrobotMapa1440 (página 6b, 2026-11-09, implementada direto e sincronizada no canvas, com o Chronicle Link 1.5); o cartão Relógios, V6MicrobotRelogios1440 (página 6c, 2026-11-09, idem, com o Chronicle Link 1.6); o painel Fazer agora, V6MicrobotFazer1440 (página 6d, 2026-11-09, idem, com o Cat Swap 1.0); o bloco Sozinho do Fazer agora, V6MicrobotSozinho1440 (página 6e, 2026-11-10, idem, com o Chronicle Link 1.7); o painel Runs e o estado esperando um ponto seguro, V6MicrobotRuns1440 (página 6f, 2026-11-10, idem) | `/automacao` |
| Automação › Plugins (2026-11-08, doc 08 fase 3) | V6MicrobotPlugins1440; o formulário de config, V6MicrobotConfig1440 (página 5b) | `/automacao/plugins` |

Onde o código diverge do canvas, por falta de dado ou de API (para o dono decidir se o canvas muda ou
se a API vem depois):

1. **Sessão e recorrentes** não foram implementados: não há API de sessão (diff entre snapshots);
   recorrentes com prazo seguem como proposta (decisão 31 do doc 01). A Automação entrou em
   2026-11-08 com o Cockpit e Plugins (doc 08, fases 1 a 3). Rotinas não aparece (fica para depois
   das receitas), o cartão "Input humano" não existe (o Agent
   Server não informa se o dono está jogando) e o painel "Script ativo" (state machine) espera a fase 3.
   O "VNC ao vivo" só habilita com o cliente headless (o x11vnc é dele) e a 700 px ou mais; abre só
   olhando, e "Intervir com mouse e teclado" alterna com "Devolver: só olhar". Em Plugins: sem
   "Usado por" (não há rotinas) e sem presets de configuração (sem onde guardá-los ainda); o grupo e as
   chaves de cada plugin ficam no navegador; "Pausar" vira "Pausar todos", porque o Agent Server pausa
   todos os loops; a lista tem três grupos (Hub, seus, do cliente) e os do cliente ficam escondidos.
   A classe `.dot-off` (ponto cinza de parado) entrou no `pergaminho.css`: os artboards a usam, mas o
   design system não a definia. O Cockpit ganhou, implementados direto e publicados depois na página
   "5b · Automação · ver e agir, logs, conta e config" (artboards V6MicrobotJogo1440 e
   V6MicrobotConfig1440, aprovação de 2026-11-05), os painéis do jogo com a conta logada: inventário, equipamento, Ação,
   diálogo, por perto, script ativo, banco, skills e chat do jogo; e "Trocar de mundo" na faixa, "Conta do cliente" e "Log do cliente"; e o formulário de configuração em Plugins
   (no lugar do chave/valor do artboard, que fica como alternativa). "Nesta sessão" no rodapé do Agora conta as últimas 6 h de eventos, sem o link "ver". O
   Grafo entrou em 2026-11-08 (doc 04, seção 5); ele diverge do canvas em três pontos: o foco padrão é
   a primeira meta fixada, escolhida num menu do botão "Foco"; a profundidade padrão é 2 (o artboard
   mostra 3); e só há requisitos de quest no catálogo, então Barrows gloves (o exemplo do artboard)
   ainda não mostra os subquests do RFD.
2. **Equipamento no boneco pelo slot do catálogo**: o contrato do plugin não traz a posição, mas cada
   item equipável ocupa um slot só, o `equipment_slot` da wiki em `content/reference/items.json`
   (exposto em `entities.ItemForm` e em `/api/account/state`); arma de duas mãos ocupa a arma e deixa o
   escudo vazio. Rune pouch e seed vault não são lidos pelo plugin e não aparecem. Os sprites cobrem
   todo item obtível do catálogo (`tools/fetch_wiki_icons.py`); um item sem sprite na wiki aparece
   com as três primeiras letras do nome.
3. **Banco sem abas e sem "só o que a rota pede"**: o contrato não traz as abas do banco.
4. **Diaries sem lista de tarefas**: o catálogo tem só os tiers; a área aberta mostra os quatro tiers.
5. **Presets do guia** (Metas) não existem no conteúdo; a lista termina em "+ fixar qualquer entidade".
6. **Chip de skill sem o nível atual** ("43 Magic · 41"): o board não devolve o fato observado junto do
   veredito; o painel de entidade mostra o estado.
7. **"Nota pessoal"** na linha aberta da Rota não existe: nota só existe junto de uma marca.
8. **"Marcar os anteriores como feitos"** (lote com desfazer) continua como ação secundária do painel
   "Fixar outro", embora o artboard não a mostre, para não perder a função.
9. O badge `AUTH_MODE=dev` de V6Setup1440 não entrou: V6Fundamentos proíbe nome de variável de ambiente
   na UI.
10. A Rota abre o trecho do cursor numa janela (3 passos antes, 10 depois) com "mostrar tudo", como em
    V6Rota1440; trechos sem título no conteúdo recebem o nome dos locais e das quests concluídas.

## 7. Reestruturação na v7: Frota (2026-11-12)

Pedido do dono, depois de uma revisão de todas as telas em uso: não é o tema nem a paleta, é
componente, disposição, usabilidade e interatividade ("tudo genérico e low effort"). O foco do produto
passa a ser várias contas com plugins, scripts e rotinas, com cara de RuneScape e a ideia de um idle
game. O primeiro acesso em passos é ruim e deve virar lista e cadastro de contas. Mudança de servidor
vira issue depois de fechar a UX. Decisão 36 do doc 01.

A revisão que originou o pedido achou, nas telas da v6 implementadas: o Cockpit com cerca de 2.700 px
e onze painéis do mesmo peso; cabeçalhos da Automação que quebram em três linhas; texto de sistema na
tela ("doc 08", "GET /state", nome de variável, papel do Zitadel); os chips de objetivo empilhados no
centro do Agora e o texto do passo três vezes; placa escura em todo painel, sem hierarquia; um separador
de banco entre quase todos os passos da rota; a barra de 640 px estourando a largura; e classes
globais curtas do `pergaminho.css` (`.bar`, `.st`, `.tabs`, `.v`, `.meta`) colidindo com as das páginas.

### 7.1 Decisões

1. **A casa do app é a Frota**, não o passo atual de uma conta. Cada conta é uma **raia**: o quadro
   com a skill que ela treina, a atividade, a barra de andamento, o ritmo, o que vem depois e as ações.
   Para dez contas ou mais há a lista **compacta**, agrupada em Precisa de você, Jogando e Paradas.
2. **A barra lateral lista as contas.** Abrir uma conta leva à tela dela, com um cabeçalho (quadro,
   nome, o que faz agora, onde roda, Pausar e Parar) e **cinco abas**: Visão geral, Rota, Personagem,
   Cliente e Histórico. A conta se escolhe na Frota ou na barra; nenhuma tela tem seletor de conta.
3. **Automação deixa de ser uma área.** Workers vira a Frota; o Cockpit vira a aba Cliente da conta;
   Plugins e Rotinas viram telas da frota inteira.
4. **Rota junta o Agora e as Rotas** numa aba, com Passo | Rota inteira | Comparar. Revê a decisão 5
   da seção 5 no que toca à navegação; Comparar continua sendo uma vista da rota (decisão 29 do doc 01).
5. **Fila** junta Relógios, Fazer agora, Sozinho e as pausas numa linha do tempo: o que a conta vai
   fazer. **Histórico** junta Runs e Auditoria: o que ela fez. Toda conta tem uma **atividade
   principal**; uma **rotina** é quando + o que faz + se falhar + em quais contas vale.
6. **Precisa de você** reúne o que pede decisão do dono (sessão expirada, relógio pronto sem rotina,
   versão antiga, falta de lugar), cada item com a ação ao lado. Vive na Frota e na tela Atividade.
7. **Nova conta** substitui o primeiro acesso: só o nome é obrigatório no desenho; sessão do jogo,
   lugar e rota podem ficar para depois.
8. **Mundo passa a ser da frota**: uma meta mostra quais contas a perseguem e quão perto cada uma está.
9. **Peças novas** (artboard `V7Fundamentos`), sobre os mesmos tokens do Pergaminho:
   - lâmpada (no jogo, subindo, em pausa, com problema, parada), sempre junto de uma palavra;
   - quadro com o sprite da skill e o nível;
   - barra de andamento (nível em verde, rotina em ouro, pausa em cinza; subindo mostra as etapas);
   - objetivo com progresso ("240 de 1 250"), no lugar do chip de uma linha; `unknown` segue pontilhado
     e nunca vira `unmet`;
   - cartão com título em serifa e a origem do dado à direita, no lugar do painel com placa. A placa
     escura fica só para o resumo da Frota e para o cliente do jogo. Revê a decisão 3 da seção 5;
   - abas para as telas de uma conta, pílulas para filtrar lista, interruptor só para o que roda sozinho.
10. **O que sai**: placa em todo painel; texto de sistema na tela; mono de 11 px como texto corrente
    (fica para hora, id de passo e número); o separador "banco · entre X e Y" (vira a marca de banco na
    linha do passo); seletor de conta dentro das telas.

### 7.2 Artboards da página 8

| Artboard | Tela |
|---|---|
| `V7Frota1440`, `V7Frota640`, `V7FrotaCompacta1440` | Frota: raias, 640 px e a lista compacta para 12 contas |
| `V7NovaConta1440` | Nova conta |
| `V7Conta1440` | Conta › Visão geral: fazendo agora, fila, rota, hoje, worker, últimas execuções |
| `V7Rota1440`, `V7Rota640`, `V7RotaLista1440` | Conta › Rota: passo atual (1440 e a janela ao lado do jogo) e a rota inteira |
| `V7Personagem1440` | Conta › Personagem: resumo, skills, quests, diaries, com ela agora, conquistas, banco |
| `V7Cliente1440`, `V7Cliente640` | Conta › Cliente: o jogo no centro; script, por perto e banco ao lado |
| `V7Historico1440` | Conta › Histórico: execuções, níveis, ações do dono e problemas, por dia |
| `V7Atividade1440` | Atividade: precisa de você, execuções da frota, XP do dia por conta, diário |
| `V7Rotinas1440` | Rotinas: atividade principal por conta, rotinas e a rotina aberta |
| `V7Plugins1440` | Plugins: o que joga, o que as rotinas usam, o que fica sempre ligado; plugin aberto |
| `V7Mundo1440` | Mundo: metas da frota |
| `V7Fundamentos`, `V7Estados1440` | As peças novas e os estados (vazio, sem dado, problemas) |

Sem desenho na v7, valendo a v6: Comparar rotas (`V6Comparar1440`), Mundo › Entidades e Grafo
(`V6Mundo*`, `V6Grafo*`, `V6Entidade`), sessão (`V6Sessao1440`), os menus do cliente do jogo
(`V6MicrobotClienteEstados1440`, `V6MicrobotUsarEm1440`, `V6MicrobotMapa1440`) e a configuração de
plugin por formulário (`V6MicrobotConfig1440`). Só Frota, Rota e Cliente têm 640 px. Os artboards
foram conferidos renderizados no tema pergaminho; no pedra, só parte deles.

A página 7 (`V6MicrobotWorkers1440`, `V6MicrobotWorkersEstados1440`) fica superada pela Frota: a lista
de workers é a própria Frota, e os estados de iniciar, subir e parar estão na raia e em `V7Estados1440`.

### 7.3 O que é mock

Contas, níveis, ritmos, ganhos, plugins e rotinas dos artboards são fictícios. Os passos da rota são
os reais do BRUHsailer (CH1.33 a CH1.41), com a atribuição.

### 7.4 O que a v7 pede do servidor

| Para | Falta |
|---|---|
| "240 de 1 250" no objetivo | o board devolver o valor observado junto do veredito (divergência 6 da seção 6) |
| XP por hora, ganhos da sessão, "Hoje", GP da conta e da frota | uma série por conta derivada dos snapshots que já existem |
| Fila e Rotinas | rotinas, gatilhos e histórico no servidor (doc 11, F6) e a noção de atividade principal por conta |
| Histórico e Atividade | runs fora do disco do agent (doc 11, F6), eventos de nível, auditoria por conta |
| Plugins da frota | perfis no Postgres (doc 11, F5) |
| Capacidade no resumo, "cabe mais N", uso por worker | doc 11, F7 |
| Mundo da frota | metas e vereditos de várias contas numa consulta |
| Nova conta sem rota e conta só acompanhada | já previstas na decisão 34 do doc 01 (modo `regular`, rota opcional) |

### 7.5 Ordem de implementação

Item 20 da sequência do doc 04, seção 9. Cada fatia entra sozinha e deixa o app inteiro; nenhuma
mostra número que o servidor não mede (sem dado, a linha some).

| Fatia | Entrega | Depende de |
|---|---|---|
| V1 | Casca: Frota em `/` com o que a API já tem, cabeçalho da conta com as cinco abas apontando para as telas atuais, Nova conta no lugar do `/setup`, barra lateral e barra de 640 px, estados vazios | nada |
| V2 | Conta › Rota: Passo, Rota inteira e Comparar numa aba; objetivo com progresso, passo a passo com caixas, marca de banco na linha | o valor observado no board |
| V3 | Conta › Cliente: o Cockpit recomposto (jogo no centro; script, por perto, banco); Relógios e Fazer agora passam para um cartão provisório na Visão geral | nada |
| V4 | Conta › Personagem e Visão geral com o que já existe (rota, worker, relógios) | nada |
| V5 | Ritmo: XP por hora, ganhos, "Hoje" e GP na raia, na Visão geral e no resumo da Frota | a série por conta |
| V6 | Rotinas e Fila | doc 11, F6 |
| V7 | Histórico e Atividade | doc 11, F6, e os eventos de nível |
| V8 | Plugins da frota | doc 11, F5 |
| V9 | Capacidade no resumo e Frota compacta | doc 11, F7 (a compacta não depende) |
| V10 | Mundo da frota e as telas que faltam em 640 px | metas de várias contas |

Antes de cada fatia que mexe numa tela ainda sem desenho na v7 (Comparar, Entidades, Grafo), o desenho
entra no canvas e o dono aprova (regra 11 do `CLAUDE.md`).

Estado em 2026-11-12: a V1 está no branch `ui/v7-casca`, em revisão: Frota em `/`, cabeçalho de conta
com as abas Visão geral, Rota, Personagem e Cliente (Histórico entra com a V7), uma Visão geral com
rota, relógios, worker e fontes, Nova conta e a barra lateral com Frota, Mundo e as contas. Diverge do
artboard de Nova conta em dois pontos, pelo que o servidor aceita: a rota é obrigatória e a sessão vem
depois de criar a conta. As telas de rota ainda têm as abas antigas abaixo dos atalhos novos (some na
V2) e a barra de 640 px não tem o Mundo. O mesmo branch pôs a aba Workers nas classes do Pergaminho e
corrigiu a barra de filtros do Grafo.
