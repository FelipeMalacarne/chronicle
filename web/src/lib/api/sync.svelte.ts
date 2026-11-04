export type SyncState = 'idle' | 'loading' | 'ok' | 'stale' | 'offline';

/**
 * Indicador de sincronização, compartilhado pela barra superior.
 * `stale` = a última chamada falhou mas já tivemos resposta antes;
 * `offline` = nunca conseguimos falar com o servidor.
 */
export const sync = $state({
	state: 'idle' as SyncState,
	lastOkAt: null as number | null, // epoch ms (número, não Date, para ser reativo sem SvelteDate)
	lastError: null as string | null,
	inflight: 0
});

export function markOk() {
	sync.lastOkAt = Date.now();
	sync.lastError = null;
	sync.state = 'ok';
}

export function markFail(err: unknown) {
	sync.lastError = err instanceof Error ? err.message : String(err);
	sync.state = sync.lastOkAt ? 'stale' : 'offline';
}
