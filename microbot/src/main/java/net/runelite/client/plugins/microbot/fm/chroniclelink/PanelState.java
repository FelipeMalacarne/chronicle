package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;

/** Immutable snapshot of the route panel, handed from the panel executor to the Swing thread. */
final class PanelState
{
	enum Connection
	{
		/** No server URL (or no token in token mode). */
		NOT_CONFIGURED,
		/** Configured, nothing fetched yet. */
		CONNECTING,
		/** The window is the planner's latest answer. */
		ONLINE,
		/** The last call failed; the window is the last one fetched ("desatualizado"). */
		STALE,
		/** The last call failed and nothing was ever fetched. */
		OFFLINE
	}

	final Connection connection;
	/** The window to show, or null. */
	final RouteWindow window;
	/** When the window was fetched, or null. */
	final Instant fetchedAt;
	/** What went wrong with the last call (PT-BR), or null. */
	final String message;
	/** The step reference Desfazer would undo, or null when there is nothing to undo. */
	final String undoRef;

	PanelState(Connection connection, RouteWindow window, Instant fetchedAt, String message, String undoRef)
	{
		this.connection = connection;
		this.window = window;
		this.fetchedAt = fetchedAt;
		this.message = message;
		this.undoRef = undoRef;
	}

	boolean stale()
	{
		return connection == Connection.STALE;
	}
}
