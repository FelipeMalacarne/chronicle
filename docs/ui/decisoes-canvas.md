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
6. **Chip de skill sem o nível atual** ("43 Magic · 41"): o board não devolvia o fato observado junto do
   veredito; o painel de entidade mostra o estado. Desde 2026-11-13 o board devolve `observed` em cada
   objetivo numérico (nível, unidades de item, quest points, kc); falta o chip usar.
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
| "240 de 1 250" no objetivo | **feito no servidor** (2026-11-13): cada objetivo numérico do board traz `observed` (`{value, target}`) dos fatos atuais da conta; a web usa desde a V2 (seção 7.10) |
| XP por hora, ganhos da sessão, "Hoje", GP da conta e da frota | **feito no servidor** (2026-11-13): `GET /api/account/rhythm?account=&since=` e, da frota com os totais, `GET /api/rhythm?since=`, derivados dos snapshots (`internal/core/rhythm`): a skill que treina (a que mais ganhou nos últimos 30 min, parada depois de 10 min sem ganho) com XP/h e o tempo até o próximo nível; a sessão (fim numa pausa de 20 min entre pushes do plugin); o dia por skill desde `since` (a meia-noite do navegador); GP = moedas e platinum tokens no banco e no inventário, com o ganho do dia. Os ganhos de item da sessão também (2026-11-13): `session.gained`, o que a conta tem a mais desde o começo da sessão, somando banco, inventário e equipamento (a forma noted junto da normal), e `session.running_out`, o que a sessão vai gastando e quando acaba nesse ritmo ("feathers acabam em 3 h"); os dois são `null` sem o banco e o inventário conhecidos nas duas pontas. O "Até a pausa" vem do Break Handler pela ponte (`/bridge/break`, PR #28, no `status` do cliente). A web usa tudo isso desde a V5 (seção 7.14) |
| Fila e Rotinas | gatilhos e histórico no servidor: **feitos** (doc 11, F6, 2026-11-13; `GET`/`POST /api/microbot/triggers`, `GET /api/microbot/runs`); rotinas recorrentes pelo relógio: **feitas na API** (doc 11, seção 16.4; `GET`/`POST`/`DELETE /api/microbot/recurring`) e **na tela** (fatia V6, seção 7.13); falta a noção de atividade principal por conta e o loadout com banco antes do run |
| Histórico e Atividade | runs fora do disco do agent: **feito** (doc 11, F6, 2026-11-13); a auditoria por conta existe (`GET /api/microbot/audit?account=`, com o que o executor faz sozinho). Eventos de nível **feitos no servidor** (2026-11-13): `GET /api/account/levels?account=&since=&limit=` (os níveis da conta, do mais novo ao mais antigo, com o total da janela e o avanço de cada skill, "Fishing 71 → 76 +412k") e, da frota do usuário, `GET /api/levels?since=&limit=` (o Diário), derivados dos snapshots (`internal/core/activity`): o nível sai do XP, um evento por nível na primeira observação que o mostrou; XP nunca cai (um valor menor que chega depois, o hiscores atrasado, é velho); a primeira observação de uma skill não é evento; `since` até 7 dias atrás, padrão as últimas 24 h. A auditoria por conta já existe (`GET /api/microbot/audit?account=`). A web usa tudo isso desde a V7 (seção 7.9) |
| Plugins da frota | perfis no Postgres (doc 11, F5) |
| Capacidade no resumo, "cabe mais N", uso por worker | **feito** (doc 11, F7, PR #25): `capacity` e `workers[].usage` em `GET /api/workers`, e `session.login_failed` para o login recusado; a web usa desde a V9 (seção 7.7). O uso do nó não existe |
| Mundo da frota | feito no servidor em 2026-11-13: `GET /api/fleet/goals` traz numa consulta as metas de todas as contas do usuário (e as chaves de `keys=`), com o veredito guardado de cada conta, a meta dela, o resumo do plano (requisitos, níveis que faltam, passos até a rota fazer) e a conta mais perto; a tela é a da V10 (seção 7.8). O "tem 132" dos quest points espera o valor observado (primeira linha), e a meta de item (Barrows gloves) só tem requisitos quando o catálogo souber de que quest o item é recompensa |
| Nova conta sem rota e conta só acompanhada | já previstas na decisão 34 do doc 01 (modo `regular`, rota opcional) |

### 7.5 Ordem de implementação

Item 20 da sequência do doc 04, seção 9. Cada fatia entra sozinha e deixa o app inteiro; nenhuma
mostra número que o servidor não mede (sem dado, a linha some).

| Fatia | Entrega | Depende de |
|---|---|---|
| V1 | Casca: Frota em `/` com o que a API já tem, cabeçalho da conta com as cinco abas apontando para as telas atuais, Nova conta no lugar do `/setup`, barra lateral e barra de 640 px, estados vazios | nada |
| V2 | Conta › Rota: Passo, Rota inteira e Comparar numa aba; objetivo com progresso, passo a passo com caixas, marca de banco na linha | o valor observado no board. **Feita** (2026-11-13, seção 7.10) |
| V3 | Conta › Cliente: o Cockpit recomposto (jogo no centro; script, por perto, banco); Relógios e Fazer agora passam para um cartão provisório na Visão geral | nada. **Feita** (2026-11-13, seção 7.11) |
| V4 | Conta › Personagem e Visão geral com o que já existe (rota, worker, relógios) | nada. **Feita** (2026-11-13, seção 7.12) |
| V5 | Ritmo: XP por hora, ganhos, "Hoje" e GP na raia, na Visão geral e no resumo da Frota | a série por conta (feita no servidor, `GET /api/account/rhythm` e `GET /api/rhythm`). **Feita** (2026-11-13, seção 7.14) |
| V6 | Rotinas e Fila | doc 11, F6. **Feita** (2026-11-13, seção 7.13), sem a atividade principal, os horários e o banco antes do run |
| V7 | Histórico e Atividade | doc 11, F6 (os eventos de nível já estão no servidor). **Feita** (2026-11-13, seção 7.9) |
| V8 | Plugins da frota | doc 11, F5. **Feita** (2026-11-13, seção 7.6) |
| V9 | Capacidade no resumo e Frota compacta | doc 11, F7 (a compacta não depende). **Feita** (2026-11-13, seção 7.7) |
| V10 | Mundo da frota e as telas que faltam em 640 px | metas de várias contas (feito no servidor, `GET /api/fleet/goals`). **Feito o Mundo da frota** (2026-11-13, seção 7.8), com as divergências do que a API ainda não mede; as telas que faltam em 640 px esperam desenho no canvas |

Antes de cada fatia que mexe numa tela ainda sem desenho na v7 (Comparar, Entidades, Grafo), o desenho
entra no canvas e o dono aprova (regra 11 do `CLAUDE.md`).

Estado em 2026-11-12: a V1 está no branch `ui/v7-casca`, em revisão: Frota em `/`, cabeçalho de conta
com as abas Visão geral, Rota, Personagem e Cliente (Histórico entra com a V7), uma Visão geral com
rota, relógios, worker e fontes, Nova conta e a barra lateral com Frota, Mundo e as contas. Diverge do
artboard de Nova conta em dois pontos, pelo que o servidor aceita: a rota é obrigatória e a sessão vem
depois de criar a conta. As telas de rota ainda têm as abas antigas abaixo dos atalhos novos (some na
V2) e a barra de 640 px não tem o Mundo. O mesmo branch pôs a aba Workers nas classes do Pergaminho e
corrigiu a barra de filtros do Grafo.

### 7.6 Implementação: V8, Plugins da frota (2026-11-13)

Artboard seguido: `V7Plugins1440` (página 8), em `/plugins` e no item "Plugins" da barra lateral (só com o
módulo Microbot ligado). O modelo atrás da tela foi decidido pelo dono em 2026-11-13:

- **O "Padrão da frota" é o perfil modelo `frota`; cada conta tem o seu perfil sobre ele**, e o ajuste
  próprio da conta fica nesse perfil. Ao instalar na frota, as contas que ainda não têm perfil ganham o
  seu, sobre o modelo.
- **Os grupos saem do uso**, nada marcado à mão: o que um gatilho roda é "Rotinas usam", o que um perfil
  liga no boot é "Sempre ligados", o resto é "Jogam".

Os dados vêm de `GET /api/fleet/plugins`: o modelo, o perfil de cada conta e o estado ao vivo de cada
cliente ligado (versão instalada, ligado ou não), mais a versão mais nova do Hub. As escritas usam as
rotas de perfil e o `POST /api/accounts/client/apply` (doc 11, seções 15 e 17).

Divergências do artboard:

1. **Ícone:** o artboard põe o sprite de um item do jogo em cada plugin; não há essa ligação no
   catálogo, então todos usam o mesmo glifo.
2. **Descrição:** no lugar da descrição do plugin, a linha diz de onde ele vem ("Plugin Hub",
   "deste repositório", "do cliente") e se está no padrão da frota. A descrição do Hub aparece na busca.
3. **Configuração:** o formulário é o da v6 (`ConfigForm`, a partir do esquema que o próprio plugin
   exporta), com "Gravar" por campo, e não os campos do artboard com um "Salvar" único. Ele precisa de um
   cliente ligado, porque o esquema vem do plugin; sem cliente, a tela diz isso.
4. **"Nas contas":** um plugin do modelo vale para todas as contas sobre ele, então a caixa fica marcada
   e travada; desmarcar só existe para o que está no perfil de uma conta. O tempo ("42 min") não aparece:
   o servidor não sabe há quanto tempo o plugin roda.
5. **Lâmpadas:** verde quando roda, dourada quando está no cliente mas parado; sem cliente ligado, a
   lâmpada fica neutra. O azul ("subindo") do artboard não tem dado.
6. **"Atualizar nas N"** fixa a versão nova no perfil e aplica nos clientes ligados; "na próxima pausa"
   não existe, porque não há como saber da pausa. **"Aplicar agora"** foi acrescentado, porque mudar um
   perfil só vale no próximo início do cliente.
7. **"Remover da frota"** tira o plugin dos perfis; o cliente o mantém até ser removido nele.

Conferido com uma captura a 1440 contra um banco descartável com um modelo e duas contas (uma com
perfil próprio), e com os testes de `node --test` da lógica da tela e o teste da rota no Postgres.

### 7.7 Implementação: V9, capacidade no resumo e Frota compacta (2026-11-13)

Artboards seguidos: `V7Frota1440` (o resumo, a raia de Nova conta e o "Precisa de você"),
`V7FrotaCompacta1440` (a lista compacta) e, para os pedidos de sessão e de lugar, `V7Estados1440`
("Sessão expirada" e "Não cabe no saradomin"). Os dados vêm de `GET /api/workers` (doc 11, F7):
`capacity` (quantos workers ainda cabem na quota e o que limita), `workers[].usage` (o que o
metrics-server mediu de cada pod) e `workers[].session.login_failed` (o jogo recusou o último login
com a sessão guardada). A lógica fica em `web/src/lib/progress/capacidade.ts` e `frota.ts`, com testes
de `node --test`.

Divergências dos artboards:

1. **Onde fica o "3 / 4 workers":** no artboard, o bloco com o número de workers, os lugares e "cabe
   mais 1 · zaros com 1 cliente" fica no pé da barra lateral. Aqui ele é um item do resumo da Frota
   ("Workers 2 de 4 no saradomin", um quadrinho por lugar da quota com o tom da conta que o ocupa, os
   livres vazios, e a mesma linha), onde a V1 já punha os workers: a barra aparece em toda tela e teria
   de ler `/api/workers` em todas, e o pé dela tem as Fontes da conta aberta. O título diz quanto um
   worker pede e o que limita, como no artboard. Sem a quota (WORKERS desligado ou quota ilegível), o
   item mostra só quantos workers rodam, sem total nem lugares.
2. **CPU e RAM:** o artboard mostra o uso do nó ("CPU 3,1 / 4", "RAM 9,1 / 15,4"). O servidor não lê os
   nós do cluster, então as barras mostram a soma do que os pods dos workers usam (metrics.k8s.io)
   contra o teto da quota (`requests` de CPU e memória do namespace), e o título diz que não é o uso do
   nó. O rótulo segue "saradomin", como em `V7Frota1440` (a compacta diz "Workers"). Sem medição ou
   sem teto, a barra some; nunca aparece um zero que o servidor não mediu.
3. **Sessão recusada:** o servidor só sabe que o jogo recusou o login, não que a sessão expirou. A
   raia diz "Login recusado", com quando e o motivo do jogo ("o jogo recusou o login há 3 h · Invalid
   credentials"), e o pedido diz "não entra: o jogo recusou o login". Um login que entra depois, ou o
   plugin mandando agora, apaga o aviso.
4. **Sem "Renovar sessão":** o botão do artboard leva à Nova conta, mas a sessão só vem do launcher, no
   zaros, pelo `chronicle accounts connect`. No lugar, o pedido traz a orientação do artboard ("Abra o
   launcher no zaros e envie a sessão de novo.") e um "Copiar comando", que copia o comando com o
   personagem da sessão, como em `V7Estados1440`; o comando fica no título do botão, não em um bloco de
   código, e o "Esperando a sessão chegar…" não aparece (não há sinal de sessão a caminho). A raia e a
   linha da compacta têm o mesmo botão, no lugar do "Renovar sessão" e do "Renovar". A conta com worker e
   sem sessão ganhou o mesmo tratamento: antes o comando aparecia no parágrafo (decisão 10 da seção 7.1).
5. **Falta lugar:** o pedido "não subiu: falta lugar" (pod `blocked` pela quota) traz o número do
   artboard ("O worker pede 0,7 núcleo e sobram 0,3. Pare outra conta ou deixe esta na espera."), mas sem
   "Escolher uma conta para parar" e "Esperar lugar": parar segue nas raias, e esperar é não fazer nada.
6. **Nova conta:** a raia diz "Cabe mais N worker(s) no saradomin; contas só acompanhadas não ocupam
   lugar." e a etiqueta "N lugar(es) livre(s)", como no artboard, só com a quota conhecida; sem ela fica
   o texto da V1.
7. **Uso por worker:** vai no título da etiqueta de onde a conta roda ("pod · … · CPU 0,4 · RAM 837 MiB"),
   no lugar do nome do pod e da imagem do artboard.
8. **Lista compacta:** a coluna Nível (barra até o próximo nível) vira Rota (o andamento da rota ativa,
   como na raia) e a coluna Ritmo sai: XP por hora, ETA de nível, itens e GP entram com a V5, nas raias e
   na compacta. "Depois" mostra só o primeiro relógio, sem sprite. Ações: Ver o jogo, Iniciar (com
   confirmação) e Copiar comando; Pausar e "Mais ações" ficam de fora (não há pausa na API; Parar e
   Reiniciar seguem nas raias). Os grupos são Precisa de você (conta com problema ou com um pedido grave,
   como a falta de sessão), Jogando (no jogo ou subindo) e Paradas; o campo filtra pelo nome.
9. **Sem o painel "Precisa de você" na compacta:** como no artboard, a compacta não tem a coluna da
   direita; o artboard manda os pedidos e o diário para a tela Atividade (V7). Até lá, os pedidos leves
   (versão antiga, relógio pronto) só aparecem nas raias.
10. **Barra lateral:** o artboard diz que, na compacta, a barra mostra só as contas fixadas e as que
    precisam de você. Não há como fixar conta, então a barra segue com todas.
11. **Raias | Compacta:** o seletor aparece nas duas vistas (no artboard das raias, no lugar dele, há o
    filtro Todas | Jogando | Paradas, que a V1 não fez) e só com alguma conta. A escolha fica neste
    navegador (`localStorage`, chave `chronicle.frota.densidade`), e um navegador que não deixa gravar
    volta às raias.

Conferido com `pnpm check`, `pnpm lint`, `pnpm test` e `pnpm build`; a captura das telas fica para depois.

### 7.8 Implementação: V10, Mundo da frota (2026-11-13)

Artboard seguido: `V7Mundo1440` (página 8), a aba Metas do Mundo, em `/mundo/metas`; o item "Mundo" da
barra lateral (e um ícone na barra de 640 px) abre nela. O cabeçalho é o da v7 (título, a frase abaixo,
as abas Metas · Entidades · Grafo nessa ordem e a busca, que abre Entidades com o texto); Entidades e
Grafo continuam as telas da v6 (sem desenho na v7, seção 7.2).

Os dados vêm de uma leitura só, `GET /api/fleet/goals`: as metas que qualquer conta do usuário fixou,
com o veredito guardado de cada conta, a meta dela, o resumo do plano e a conta mais perto. "O que
falta" lê o plano da meta da conta (`GET /api/goals/{id}/plan`); "fixar para ela" é o `POST /api/goals`
dessa conta; Desafixar tira a meta de cada conta que a persegue. A lógica da tela está em
`web/src/lib/progress/mundo.ts`.

Divergências do artboard:

1. **Endereço:** o título do artboard no canvas aponta `/mundo`; a tela fica em `/mundo/metas` e `/mundo`
   continua sendo Entidades, para não quebrar a busca (⌘K) nem os links `?e=` do painel de entidade.
   A barra lateral e o antigo `/goals` levam às Metas.
2. **Quest points:** "falta 175 QP (tem 132)" vira "falta 175 QP": o servidor ainda não tem o valor
   observado dos quest points (`short[].level` é sempre nulo em `qp`). Pelo mesmo motivo a linha
   "175 quest points" de "O que falta" fica sem o número à direita.
3. **Subquests:** a contagem é a das quests requisito sabidamente não feitas, em qualquer nível do
   plano; a linha diz "quests".
4. **Meta de item:** Barrows gloves e as outras metas de item não têm requisitos no catálogo (falta
   saber de que quest o item é recompensa, seção 7.4). Sem medida, a linha da conta fica sem barra e
   sem porcentagem, a lista não diz "a mais perto", "O que falta" diz que não há requisitos conhecidos
   e o cabeçalho não tem o "recompensa de Recipe for Disaster".
5. **"O que falta":** o requisito feito não diz de onde veio a evidência ("hiscores", "plugin"), porque
   o plano não traz a fonte, e "iniciada" não existe, porque o plano não sabe de quest começada. A
   bolinha meio cheia é a de um requisito com parte dos seus requisitos feitos; o presumido pela marca
   diz "pela marca" e o disponível diz onde a rota ativa o faz. Uma meta de nível mostra a linha do
   próprio nível.
6. **A conta de "O que falta"** é a mais perto entre as que perseguem a meta; o nome de outra conta que
   a persegue, em "Na frota", troca a conta. Quem não persegue não tem plano.
7. **"os N requisitos"** abre o plano inteiro da meta daquela conta, que é o da v6 (`V6Metas1440`, a
   meta aberta), em `/mundo/metas/plano`: a v7 não redesenhou o plano.
8. **Desafixar** com mais de uma conta perseguindo pede uma segunda confirmação ("Tirar das N
   contas"), porque tira a meta de todas.
9. **Lâmpada** da conta: o estado que a Frota calculou, como na barra lateral; antes de passar pela
   Frota, fica neutra.
10. **Abas:** o controle das abas do Mundo da v6, um pouco mais alto que o segmentado do artboard.
11. **640 px:** não há artboard do Mundo a 640 px; a tela só empilha as colunas e a linha da conta
    quebra em duas. As outras telas da v7 sem 640 px (seção 7.2) continuam pendentes.
12. **Sem meta:** o estado vazio não está desenhado (nem em `V7Estados1440`); a lista diz que nenhuma
    conta fixou uma meta e mantém "Fixar outra meta", e a direita fica vazia.

Conferido com os testes de `node --test` da lógica da tela, o `svelte-check` contra a fixture do
contrato de `GET /api/fleet/goals`, `pnpm lint` e `pnpm build`; a captura a 1440 fica para a revisão.

### 7.9 Implementação: V7, Histórico e Atividade (2026-11-13)

Artboards seguidos: `V7Historico1440` (página 8), na aba Histórico da conta (`/historico`), e
`V7Atividade1440`, em `/atividade` e no item "Atividade" da barra lateral, logo abaixo da Frota, como no
artboard. A Atividade é da frota: sem o cabeçalho da conta (decisão 1 da seção 7.1).

Os dados: os níveis de `GET /api/account/levels` (7 dias, o avanço de cada skill no cartão Na semana) e
de `GET /api/levels` (o Diário, desde a meia-noite do navegador); os runs de `GET /api/microbot/runs`
(os 30 mais novos de cada conta) e as ações de `GET /api/microbot/audit` (as 200 mais novas da conta),
os dois só com o módulo Microbot ligado; o XP de hoje do ritmo (`GET /api/account/rhythm` e
`GET /api/rhythm`); Precisa de você é a mesma lista da Frota (worker e relógios de cada conta). O que
um run subiu ("Fishing 74 → 75") são os níveis da conta observados entre o começo e o fim dele, e o
sprite do run é o da skill que mais subiu; sem nível no caminho, o run fica com o glifo.

Divergências dos artboards:

1. **Tempo no jogo:** "3 h 13 min no jogo" no resumo do dia e "No jogo 31 h" no cartão Execuções não
   aparecem: o servidor não mede o tempo logado. No lugar de "No jogo", o cartão mostra as canceladas,
   quando há.
2. **XP por dia:** só hoje tem XP no resumo (o ritmo mede desde a meia-noite); os outros dias dizem
   quantos níveis tiveram, as execuções e os problemas.
3. **GP na semana:** a linha "GP 1,9M +140k" do cartão Na semana fica de fora: o ritmo só sabe o ganho
   de GP do dia.
4. **Nome do run:** a linha diz o plugin que rodou ("Cat Swap", tirado da classe), não o nome da rotina
   ("Farm run"), e "pela rotina" no lugar de "pelo relógio", porque o run não diz se foi o relógio ou o
   intervalo que o disparou. O sprite de item do artboard não existe (sem ligação entre plugin e item);
   vale o da skill que o run subiu.
5. **Eventos do worker:** "Worker subiu no saradomin · sessão entregue · cliente na tela de login" não
   existe; o Histórico mostra só o que a auditoria guarda: o dono iniciar, parar ou reiniciar o worker e
   enviar ou apagar a sessão. No Diário da Atividade, pelo mesmo motivo e porque não há evento de quest,
   só entram os níveis ("terminou Jungle Potion" e "subiu no saradomin" ficam de fora).
6. **"Abrir a rotina":** o link do cartão Execuções fica de fora até a tela Rotinas (V6) existir.
7. **Ações de Precisa de você:** "Renovar sessão" e "Deixar parada", "Rodar Farm run agora" e "Ligar a
   rotina nela" não existem: a sessão vem do launcher (`chronicle accounts connect`) e o relógio pronto
   não sabe qual rotina cuidaria dele. Cada item tem "Abrir a conta"; a versão antiga tem também
   "Reiniciar agora" (com confirmação, porque sai do jogo). "Reiniciar na pausa" espera o servidor
   saber da pausa (resolvido na seção 7.15: o servidor reinicia sozinho, sem botão).
8. **Número na barra lateral:** o "3" ao lado de Atividade (quantos pedem decisão) fica de fora: a barra
   não lê os workers e os relógios de toda conta em toda tela.
9. **Execuções da Atividade:** são as de hoje, mais as que começaram antes e ainda rodam; como o
   servidor devolve 30 runs por conta, uma conta com mais de 30 num dia mostra só as 30 mais novas. No
   Histórico, quando os 30 runs cabem todos na janela, o cartão diz "últimas 30" em vez de "7 dias".
10. **Filtros sem o Microbot:** sem o módulo, o Histórico fica com Tudo e Níveis, e a Atividade sem
    Execuções; o resto da tela segue.

Conferido com `pnpm check`, `pnpm lint`, `pnpm test` (a lógica das duas telas em `historico.ts` e
`atividade.ts`, com `node --test`) e `pnpm build`; sem captura de tela nesta fatia.

### 7.10 Implementação: V2, Conta › Rota (2026-11-13)

Artboards seguidos: `V7Rota1440`, `V7Rota640` e `V7RotaLista1440` (página 8), na aba Rota: o Passo em
`/agora`, a Rota inteira em `/rotas/<guia>/<seção>` e Comparar em `/compare`, que não tem artboard na v7
e segue `V6Comparar1440` com a barra nova em cima. A barra da rota (seletor, capítulo com o andamento,
Passo | Rota inteira | Comparar e Sincronizar) substitui as abas antigas.

Divergências do artboard:

1. **GP e tempo do guia** saem das notas do BRUHsailer ("GP after step", "Total time"); a Oziris não
   tem, e no lugar o cabeçalho diz onde o passo é.
2. **"240 / 1.250"** só aparece quando o board traz o valor observado; sem ele, o objetivo fica com a
   quantidade no nome e só o veredito.
3. **O anel de ouro** vai na primeira ação que falta (`unmet`). No artboard ele está numa ação "não
   sabemos", que nunca vira falta.
4. **O passo a passo com caixas** só existe nos passos-parágrafo do BRUHsailer com mais de uma ação.
5. **Ficam da v6**, sem desenho na v7: as notas do autor e os links no cartão do passo; os grupos Requer
   e Usa; a prévia de Levar do banco (15, 30 ou até o banco) e os estados dele ("espelho restaurado",
   "sem leitura do plugin"); a faixa de fonte do hiscores; Rota concluída; o diálogo de Fixar outro
   passo. O passo sem objetivo segue `V7Estados1440`.
6. **Aqui perto** só aparece com a posição do plugin (a opção "Send position" do Chronicle Link) e
   **Relógios** some sem relógio contando.
7. **Seletor de rotas:** o artboard só desenha o botão. O menu traz cada rota com o andamento, onde ela
   está e Tornar ativa; o Passo mostra sempre a rota ativa.
8. **Em seguida** fica no capítulo do passo: no último passo de um capítulo, a lista some.
9. **Levar do banco** a 640 tem o arranjo da 1440 (o artboard põe a grade antes e o texto numa linha).
10. **Rota inteira:** a linha abre por dentro, como na v6, em vez de levar ao Passo; a janela tem 3
    passos antes do atual e 10 depois (o artboard, 1 e 7); "quem fez" vem das últimas 60 marcas, e as
    mais antigas ficam sem; ficam da v6 a faixa "Você está em", a busca em outros capítulos, os
    presumidos e as camadas Enhanced e HCIM.
11. **Comparar** fica com a barra sem capítulo nem andamento.

Conferido com os testes de `node --test` da lógica da tela (`objetivo.ts`, `rota.ts`, `agora.ts`,
`relogios.ts`), `pnpm check` e `pnpm build`; as capturas ficam para a revisão.

### 7.11 Implementação: V3, Conta › Cliente (2026-11-13)

Artboards seguidos: `V7Cliente1440` e `V7Cliente640` (página 8), na aba Cliente (`/automacao`), que só
existe com o módulo do Microbot ligado no servidor. O Cockpit da v6 vira a aba: a faixa com o estado do
cliente, Imagem | Ao vivo, o mundo e as ações; o aviso de script jogando com Pausar o script; o jogo no
centro (a tela, o chat e o painel de itens com o menu do jogo, e o banco aberto no lugar da tela); ao
lado, os cartões Script, Por perto e Banco. A 640 empilha a tela, o chat, o painel e Por perto, sem ao
vivo e sem trocar de mundo. Os menus do jogo (Choose Option, Usar em, o mapa do mundo) seguem a v6
(`V6MicrobotClienteEstados1440`, `V6MicrobotUsarEm1440`, `V6MicrobotMapa1440`), como diz a seção 7.2.

Saem do Cliente o painel "Estado · GET /state" e a faixa de cartões Cliente, Conta e Agent (texto de
sistema): a posição e o mundo ficam no HUD da tela.

Divergências do artboard:

1. **Faixa:** sem o nome do lugar ("Barbarian Outpost"), porque o servidor não traduz coordenadas em
   lugar; o HUD diz "imagem a cada 5 s" no lugar da idade da imagem. **Iniciar o cliente** e **Parar o
   cliente** continuam na faixa: com o agent anexado (zaros), sem worker, o cabeçalho não tem Parar.
2. **Script:** sem o sprite da skill, porque não se sabe qual skill um plugin treina, e sem "sem erros",
   porque o heartbeat não conta erros (quando ele falta, a linha diz "sem heartbeat"). Pausar pausa
   todos os scripts, que é o que o Agent Server faz. **Configurar** leva aos plugins deste cliente
   (`/automacao/plugins`), porque Rotinas (V6) ainda não existe.
3. **Por perto:** sem o tipo "ponto": no jogo, o fishing spot é um NPC.
4. **O que o Cockpit tinha e o artboard não põe no Cliente** vai, provisório, para a Visão geral:
   Relógios e Fazer agora num cartão no lugar da Fila (até a V6), as últimas execuções, as ações no
   cliente (a antiga Auditoria, até o Histórico da V7) e o log e a conta do cliente, atrás de botões no
   cartão Worker (a conta do cliente só com o agent anexado).

Conferido com os testes de `node --test` da lógica da tela (`jogo.ts`, com as linhas do cartão Script, e
`automacao.ts`), `pnpm check` e `pnpm build`; as capturas ficam para a revisão.

### 7.12 Implementação: V4, Conta › Personagem e Visão geral (2026-11-13)

Artboards seguidos: `V7Personagem1440` na aba Personagem (`/conta`) e `V7Conta1440` na Visão geral
(`/visao`), com o que o servidor já tem. O ritmo fica para a V5, sem lugar reservado na tela.

**Personagem:** o resumo (total, combat, quests com os QP, diaries, combat achievements e collection
log; sem leitura, a célula some), Skills com a barra até o próximo nível, Quests com busca e Em
andamento | Feitas | Faltam, Diaries com as quatro bolinhas por área, Com ela agora (equipado e
inventário), Conquistas (os tiers de combat achievements e o collection log) e Banco com busca. Cada
cartão diz a fonte e a idade da leitura. Saem da tela da v6 o botão Sincronizar hiscores (fica na barra
da Rota), o cartão Fontes (as fontes ficam na barra lateral e em cada cartão), a ordem do banco e o
boneco do equipamento.

Divergências do artboard, no Personagem:

1. **O que subiu hoje** (as skills em verde e a frase embaixo da grade) entra com a V5, junto do cartão
   Hoje; a explicação da barra foi para o `title` de cada skill.
2. **Quests:** "3 de 10 partes" não existe, porque o plugin só diz se a quest foi iniciada.
3. **Diaries:** clicar numa área abre, no painel da entidade, o primeiro tier que falta; o artboard não
   desenha o clique.
4. **Banco:** sem o valor em GP ("vale cerca de 6,1M gp"), porque o servidor não tem preço de item. O
   cartão mostra 48 itens e "ver o banco inteiro" abre o resto; com a leitura de mais de um dia, ele
   ganha o aviso do estado "Dado velho" de `V7Estados1440`.
5. **Com ela agora:** "ao vivo" só com uma leitura de menos de 5 min; depois diz "lido há", e o espelho
   restaurado diz "espelho".
6. **Conquistas:** Master e Grandmaster só ganham barra quando têm algum feito; antes, a nota diz que
   estão sem nada, como no artboard. As páginas do collection log que o plugin já manda ficam fora, sem
   desenho.
7. Esta tela não tem 640 px no canvas; os cartões quebram em coluna.

**Visão geral:** Fazendo agora (a tela do jogo a cada 5 s, que abre o Cliente, com o mundo e a posição;
no título, o script que joga, o estado da state machine e as voltas), só com o cliente rodando, e ao lado
o cartão provisório de Relógios e Fazer agora (seção 7.11) no lugar da Fila; sem cliente, esse cartão
fica entre a Rota e o Worker. A Rota com o andamento, o passo atual e até dois objetivos. O Worker com o
pod, a versão, o uso (CPU e memória, doc 11, F7), a sessão e o plugin, e Reiniciar, Log do cliente e
Conta do cliente. Embaixo, as Últimas execuções.

Divergências do artboard, na Visão geral:

1. **Fazendo agora** fica sem a barra da skill que ela treina, XP por hora, Nesta sessão, Até a pausa e
   Ganhou nesta sessão, e o cartão **Hoje** não aparece: entram com a V5 (o servidor já mede, seção
   7.4). Sem cliente rodando, o cartão some; o cabeçalho já diz Parada ou Só acompanhada.
2. **Fila:** o cartão provisório de Relógios e Fazer agora fica até as rotinas (V6), sem Pôr na fila nem
   Editar rotinas. Pela mesma razão, a Rota não diz "feito à mão, fora da fila".
3. **Worker:** sem FPS, que não é medido, e sem Mover para o zaros, que não existe. "De pé há" vira
   "pedido há", porque o servidor guarda quando o worker foi pedido, não quando o pod subiu. Conta do
   cliente só aparece com o agent anexado.
4. **Últimas execuções:** o título diz quantas estão guardadas, no lugar de "ver o histórico completo"
   (o Histórico é a V7), e as linhas não têm o sprite da atividade.
5. **Ações no cliente** (a antiga Auditoria) num cartão embaixo, sem desenho na v7, até o Histórico.
6. **Fontes:** o cartão da V1 sai, como no artboard; o botão da faixa de fonte da Rota leva ao
   Personagem.

Conferido com os testes de `node --test` da lógica das telas (`personagem.ts` e `workerFacts` em
`workers.ts`), `pnpm check` e `pnpm build`; as capturas ficam para a revisão.

### 7.13 Implementação: V6, Rotinas e Fila (2026-11-13)

Artboards seguidos: `V7Rotinas1440` (página 8), em `/rotinas` e no item "Rotinas" da barra lateral (só
com o módulo Microbot ligado, como Plugins), e o cartão Fila do `V7Conta1440`, na Visão geral. Os dados
vêm de `GET`/`POST /api/microbot/recurring?account=` e `DELETE /api/microbot/recurring/{id}?account=`
(doc 11, seção 16.4), dos gatilhos e do histórico de runs (`/api/microbot/triggers`,
`/api/microbot/runs`) e do `status` do cliente (o break do Break Handler pela ponte).

- **Uma rotina da frota são as cópias de cada conta.** O servidor guarda as rotinas por conta; a tela
  junta as recorrentes pelo nome (único na conta) e os gatilhos pelo evento, e "Contas" diz em quais
  ela vale. Marcar uma conta cria a cópia nela; desmarcar exclui a cópia daquela conta, com o segundo
  clique. Quando as cópias diferem (relógio, intervalo, prioridade ou run), a rotina aberta mostra a da
  primeira conta e avisa que salvar põe essa em todas. Um gatilho não se exclui pela API: a conta que
  já tem o gatilho fica marcada e travada, e ele se desliga no interruptor.
- **Ligar é sempre uma escolha à parte.** Uma rotina nova, ou a cópia numa conta nova, nasce desligada;
  salvar mantém o interruptor de cada conta como estava; ligar pede o segundo clique e vale para todas
  as contas da rotina ("ligada em 2 de 3" quando só parte está). Rodar agora também pede o segundo
  clique, numa conta escolhida, com o cliente ligado e a conta logada.
- **As travas aparecem.** Um run que falhou ou foi cancelado segura a rotina naquela conta até ela ser
  salva de novo; o último run da conta, quando falhou, segura todas as rotinas dela. Cada trava vira um
  aviso no topo da lista e na rotina aberta, com Liberar (segundo clique: liberada e ligada, ela pode
  começar logo). Salvar, ou desligar, numa conta com trava também solta a trava e pede o segundo
  clique. Sem o agent, a tela calcula a trava pelo que o banco guarda, com as regras do vigia.

Divergências do artboard:

1. **Atividade principal:** o servidor não tem atividade principal por conta (seção 7.4). No lugar da
   seção "Atividade principal", a seção **Fila** mostra, por conta, a próxima rotina pronta ou com hora
   e a linha do vigia: o que ele espera ("esperando: a pausa do Break Handler começa em 8 min"),
   relógios sem leitura, agent desligado ou antigo. No "Faz", "Para a atividade principal" vira "Para"
   os plugins marcados entre os ligados no cliente, e "Religa a atividade principal" religa os mesmos.
2. **Pausas e horário:** as linhas "Pausas" (Break Handler) e "Dormir de madrugada" não existem no
   servidor. As pausas são a configuração do Break Handler (tela Plugins), e não há rotina por horário:
   só relógio, intervalo e evento. A próxima pausa aparece na Fila da Visão geral, lida da ponte.
3. **"+ Passo" e o banco antes do run:** a rotina roda um plugin só, com o parar, voltar e religar do
   Fazer agora. Passos tipados e o loadout com banco antes do run (o PREPARO do doc 08) não existem na
   API e ficam de fora.
4. **Se falhar:** "tenta de novo em 15 min e avisa em Precisa de você" não existe. O servidor segura a
   rotina até o dono salvá-la de novo; a linha diz isso, e o aviso fica no topo da tela (Precisa de
   você é da fatia V7).
5. **Campos a mais:** o nome (o servidor pede, e é ele que junta as cópias), "sem leitura do relógio, a
   cada N min" (o intervalo de reserva da recorrente pelo relógio), a prioridade (a ordem da fila do
   vigia) e o limite do plugin. O "até 60 s" do passo 1 não aparece: a espera pelo ponto seguro é do
   agent.
6. **Cabeçalho da rotina aberta:** "ligada · rodou 14 vezes" vira "ligada em N contas": o servidor
   devolve só os últimos 30 runs de cada conta, e a contagem sairia errada.
7. **Linha da rotina:** as lâmpadas são das contas, como no artboard; abaixo de "3 contas" entra quando
   a cópia mais urgente entra na fila ("Felipe Pesca · em 12 min", "segura"), com o porquê no `title`.
   A lista fica em ordem fixa (as ligadas primeiro, depois o nome); a ordem da fila de cada conta está
   na seção Fila.
8. **Fila da Visão geral:** sem a atividade principal ("agora · Tick Fisher", "depois · retoma"), o
   item de agora é o run em andamento ou a pausa do Break Handler; as rotinas prontas entram como "a
   seguir", as outras pela hora, com a próxima pausa entre elas; o "~4 min" é a duração do último run
   feito da rotina. Os gatilhos entram no fim ("no evento"), e as seguras depois. "Pôr na fila" não
   existe: um run avulso é o Rodar agora da rotina ou o Fazer agora do Cliente.

Conferido com os testes de `node --test` da lógica das telas (`rotinas.test.ts`), `pnpm check`,
`pnpm lint` e `pnpm build`. Falta a captura a 1440 e conferir contra um agent de verdade.

### 7.14 Implementação: V5, Ritmo (2026-11-13)

Artboards seguidos: `V7Frota1440` (a raia e o resumo), `V7Frota640` (a raia e o resumo a 640 px),
`V7FrotaCompacta1440` (a coluna Ritmo), `V7Conta1440` (Fazendo agora e o cartão Hoje) e, para o que
subiu hoje, `V7Personagem1440`. Os dados vêm de `GET /api/rhythm?since=` na Frota (as contas e os
totais numa leitura, a cada 30 s) e de `GET /api/account/rhythm?account=&since=` na Visão geral e no
Personagem, com `since` na meia-noite do navegador; a pausa vem do `break` do status do cliente, que
a Frota e a Visão geral já liam. A lógica fica em `web/src/lib/progress/ritmo.ts` (e a pausa da raia em
`frota.ts`), com testes de `node --test`.

**Frota, raia:** com uma skill ganhando XP, o quadro mostra essa skill e o nível, a barra é a do nível
("76", a barra até o 77, "77 em 22 min"), e embaixo vêm o XP por hora, os três itens que a sessão mais
rendeu (sprite e quantidade) e o que acaba primeiro, com o que a sessão já gastou dele ("−1,1k"); o
título de cada item diz o nome, o que ganhou ou gastou, quanto sobra e quando acaba. Sem skill
treinando, a raia segue como na V1 (a skill mais alta no quadro e a barra da rota). Num break do Break
Handler a raia fica em pausa: lâmpada dourada, "Em pausa · Break Handler · volta para Tick Fisher em
6 min", a barra cinza e o quadro parado; a conta conta como jogando. **Resumo:** "XP hoje" ("412k",
"7 níveis · 3 contas") e "GP na frota" (com a moeda e "+84k hoje"), depois de No jogo. **Compacta:** a
coluna Ritmo volta, entre Rota e Depois, com o XP por hora.

**Visão geral:** Fazendo agora mostra, ao lado da tela do jogo, a barra da skill que treina ("Fishing
76", "77 em 22 min"), XP por hora, Nesta sessão e Até a pausa (ou "Em pausa · volta em 6 min"), e
"Ganhou nesta sessão" com os itens e, por último, o que acaba primeiro ("Feather acabam em 3 h 05", na
caixa tracejada). O cartão Hoje, entre a Rota e o Worker, traz uma linha por skill que ganhou XP desde
00:00 ("Fishing 74 → 76 +96,4k"; a skill que não subiu de nível mostra só o nível) e o GP com o ganho do
dia. **Personagem:** as skills que subiram hoje ficam em verde, com a frase "Em verde, o que subiu
hoje: Fishing +2, Farming +1." embaixo da grade.

Divergências dos artboards:

1. **Sem dado, a linha some.** XP por hora só com uma janela medida (mais de 2 min de pontos); "77 em
   22 min" some no 99 e sem ritmo; os itens somem quando o banco ou o inventário não eram conhecidos nas
   duas pontas da sessão (`gained` e `running_out` nulos, que não são "nada ganho"); o GP some sem o
   banco visto, e o "+22k hoje" sem uma observação do começo do dia. "Até a pausa" só aparece com o
   cliente ligado e a ponte dizendo o tempo (o Break Handler V2 não diz).
2. **Ritmo só de quem joga.** A raia e a coluna Ritmo só mostram o ritmo com a conta jogando (no jogo,
   em pausa ou à mão) ou com XP ganho nos últimos 10 min: numa conta parada, a sessão que o servidor
   devolve é a última, não a de agora. Pelo mesmo motivo, na Visão geral Nesta sessão e os itens só
   aparecem com a conta logada, em pausa ou ganhando XP. A raia parada não diz "última: Motherlode Miner
   · +41k xp" (o artboard): o servidor não liga a sessão a um script.
3. **Fazendo agora sem cliente:** o cartão aparece também quando a conta ganha XP num cliente que o
   Chronicle não controla, só com o ritmo e "à mão" no título; com o cliente rodando e sem ritmo, fica
   só a tela, como na V4.
4. **"+65 xp" no quadro** (o XP que cai a cada ação) não aparece: o servidor mede por snapshot, não por
   ação.
5. **Nomes dos itens:** o nome do catálogo, como o jogo escreve ("Raw shark", "Feather"), e não o curto
   do artboard ("sturgeon", "feathers"). O que acaba diz "Feather acabam em 3 h 05", com o minuto.
6. **Barra da rota na raia:** o artboard não tem; a raia mostra a barra do nível quando há skill
   treinando e a da rota nos outros casos, como desde a V1. Na compacta, a coluna continua Rota (seção
   7.7, item 8), e o nível vai na coluna Skill.
7. **Coluna Ritmo da compacta:** só o XP por hora; o "parada há 19 h" e o "cliente abrindo" que o
   artboard põe nessa coluna já estão em Fazendo, e a célula fica vazia.
8. **640 px:** como em `V7Frota640`, o resumo esconde o GP; a raia mantém os itens da sessão (o
   artboard a 640 troca os itens pelo "depois:", que aqui fica no Depois, embaixo).
9. **Hoje:** até seis skills, da que mais ganhou; as outras viram "mais N skills" com o XP somado. Sem XP
   desde 00:00, o cartão diz "Nenhum XP visto desde 00:00." (o servidor só sabe do que viu). O 00:00 é
   a meia-noite deste navegador.
10. **Personagem:** a frase sobre a barra continua no `title` de cada skill (seção 7.12, item 1); embaixo
    da grade fica só "Em verde, o que subiu hoje: …", e só quando algo subiu.
11. **Cabeçalho da conta e barra lateral:** o cabeçalho mostra a pausa (lâmpada dourada, "Em pausa ·
    Break Handler · volta em 6 min"), mas o quadro segue com a skill mais alta, e não com a que ela
    treina, como em `V7Conta1440`: o cabeçalho não lê o ritmo, que seria mais uma leitura em toda aba da
    conta. A barra lateral, que usa o estado calculado pela Frota, deixa a conta em pausa com a lâmpada
    verde e sem o "pausa · volta em 6 min" do artboard.

Conferido com `pnpm check`, `pnpm lint`, `pnpm test` (`ritmo.test.ts` e `frota.test.ts`) e `pnpm build`;
as capturas ficam para a revisão.

### 7.15 Reinício na versão nova, na pausa (2026-11-13)

Decisão do dono (doc 01, decisão 37; doc 11, seção 7.2): um worker numa versão antiga se reinicia
sozinho na nova na próxima pausa (sem run, sem VNC aberto, deslogado ou no break do Break Handler).
Artboards lidos: `V7Frota1440` (o pedido "roda numa versão antiga" em Precisa de você, com "Reiniciar
sai do jogo; na pausa não perde nada." e o botão "Reiniciar na pausa"), `V7Atividade1440` (o mesmo
pedido, com "Na próxima pausa, em 6 min, não perde nada.", "Reiniciar na pausa" e "Agora") e
`V7Historico1440` (as linhas do worker, como "Worker parado · você parou · saiu do jogo").

Divergências dos artboards:

1. **Sem "Reiniciar na pausa":** o artboard pede um clique para reiniciar na pausa; o dono decidiu
   que o servidor faz isso sozinho com todo worker desatualizado, então o botão sumiu. O pedido diz
   "roda numa versão antiga" e embaixo "Reinicia sozinho na próxima pausa.". Na Atividade fica o
   "Reiniciar agora" (o "Agora" do artboard), com confirmação; na Frota, o "Atualizar" da raia. O
   título dos dois diz que o clique reinicia agora e sai do jogo, e que sem ele o worker reinicia
   sozinho na próxima pausa (o mesmo título no cabeçalho da conta e no cartão Worker da Visão geral).
2. **Sem "em 6 min":** o pedido não diz quando vem a pausa. O servidor não prevê a pausa (o próximo
   break do Break Handler pode nem existir, e o deslogado não tem hora); ele só a vê quando chega.
3. **Histórico:** o reinício automático entra como "Worker reiniciado na versão nova · sozinho, na
   pausa · deslogado" ("no break", "cliente fechado"), na forma das outras linhas do worker; o
   artboard não tem esta linha. A auditoria da Visão geral mostra "reiniciar o worker · versão nova,
   na pausa", com "servidor" no lugar de "você".

Conferido com `pnpm check`, `pnpm lint`, `pnpm test` (`frota.test.ts`, `historico.test.ts`,
`automacao.test.ts`) e `pnpm build`; sem captura de tela.

- Marca manual por objetivo (2026-11-13): implementada fora do canvas, versão mínima autorizada pelo dono; o desenho ainda precisa passar pelo agente de design.
