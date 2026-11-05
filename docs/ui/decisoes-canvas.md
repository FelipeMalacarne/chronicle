# Decisões de UI do canvas (Claude Design)

Registro das decisões tomadas na reestruturação das telas do planner no canvas do Claude Design
(<https://claude.ai/artifact/NqmKesmqCJH1gnSowjJFbQ>), com o design system **Pergaminho**
(<https://claude.ai/artifact/WrVkgXPtCrRJPeUSgpHWVC>). Complementa `brief-ui-ux.md` (brief) e segue o
modelo de domínio de `../09-modelo-de-dominio.md`. Nada aqui altera código em `web/` nem os docs
numerados.

Histórico: as seções 1 a 3 foram escritas para a página v4 (2026-11-04). Em 2026-11-05 o canvas foi
consolidado numa única página, **v6**, e as páginas v0 a v5 foram removidas (seção 5). Onde a v6
revisou uma decisão da v4, a seção 5 prevalece.

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

1. **Nome Chronicle.** Wordmark Alegreya 700 com o símbolo do livro e a runa, em ouro sobre madeira;
   "Ironman Planner" vira descrição.
2. **Limpeza sem esconder.** A v5-limpeza falhou por esconder demais (menus, banco recolhido). Na v6
   nada some: cada fato continua na tela, com menos camadas. Explicação nunca em parágrafo: vai para
   o `title` ou some.
3. **Um bisel só, o do painel.** Dentro dos painéis tudo é plano (células, chips, slots e linhas
   separados por fio); seleção é faixa dourada com barra à esquerda. Sidebar opção A (madeira
   inteira, refinada) em todas as telas de 1440 px.
4. **Dois alvos: 640 px** (janela ao lado do jogo) **e 1440 px** (olhar analítico), mesmos componentes.
5. **A lista de Rotas sai.** "Rotas" abre a rota ativa na seção do cursor, com abas Oziris ·
   BRUHsailer · Comparar. A aba da rota inativa mostra onde você estaria e "Tornar ativa".
6. **Comparar volta como aba**, revisando a decisão 1 da v4 a pedido do dono, mas fica em aberto se a
   comparação deve ser a tela principal.
7. **Mundo**: lista "no seu caminho agora" e "feitas há pouco"; o painel de entidade é o mesmo em todas
   as telas e abre por cima, sem sair da página.
8. **Conta**: skills e quests com fonte e idade primeiro, depois containers com frescor, fontes no fim.

Portado das páginas antigas para a v6 na consolidação: Agora e Rota com a BRUHsailer (1440 e 640),
Comparar, Agora com fonte falhou / Fixar outro / recorrentes, estados sem dados, sessão, primeiro
acesso e Automação (Microbot: cockpit, rotinas, plugins, 640). Os artboards portados usam a casca da
v6; passar neles o registro da v6 (menos texto de sistema) fica para a próxima iteração.
