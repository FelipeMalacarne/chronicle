<svelte:head>
	<title>Conta · Chronicle</title>
</svelte:head>

<header class="head">
	<p class="ey">Conta</p>
	<h1 class="sr">Skills, quests, containers e fontes</h1>
	<p class="mu">
		Em breve: skills, pontos de quest, diaries e bosses, alimentados pelo plugin e pelos hiscores
		(V4Conta no canvas, item 11 da sequência). Até lá, crie contas de teste com <code
			>just account-create</code
		>.
	</p>
</header>

<style>
	.head {
		display: flex;
		flex-direction: column;
		gap: 6px;
		max-width: 72ch;
	}
	h1 {
		font-size: 30px;
	}
	p.mu {
		margin: 0;
	}
	code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
	}
</style>
