package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.awt.Component;
import java.awt.Container;
import java.awt.Window;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.Supplier;
import java.util.stream.Collectors;
import javax.swing.JDialog;
import javax.swing.JOptionPane;
import javax.swing.SwingUtilities;
import lombok.extern.slf4j.Slf4j;
import net.runelite.client.plugins.microbot.Microbot;

/**
 * The client's modal message windows: the {@link JOptionPane} dialogs of {@code Microbot.showMessage}.
 * Microbot opens one when a script gives up reaching an NPC or an object, after pausing every script,
 * and the script's thread waits for its OK. On a worker nobody sees it, so the agent reads them during
 * a run and closes them (FEL-134).
 */
@Slf4j
final class ClientMessages
{
	private ClientMessages()
	{
	}

	/** The texts of the message windows on screen. */
	static List<String> open()
	{
		return onEdt(() -> texts(dialogs()));
	}

	/**
	 * Closes the message windows and returns their texts. When it closed one it also lifts Microbot's
	 * pause of every script, which the recovery that opened the window set; a pause from the toolbar is
	 * {@code ScriptPauseController}'s and stays.
	 */
	static List<String> close()
	{
		List<String> closed = onEdt(() ->
		{
			List<JDialog> dialogs = dialogs();
			List<String> texts = texts(dialogs);
			for (JDialog d : dialogs)
			{
				d.setVisible(false);
				d.dispose();
			}
			return texts;
		});
		if (!closed.isEmpty())
		{
			Microbot.pauseAllScripts.set(false);
		}
		return closed;
	}

	/** The text of a JOptionPane's message: a string, or the strings of an array; a component has none. */
	static String text(Object message)
	{
		if (message == null || message instanceof Component)
		{
			return "";
		}
		if (message instanceof Object[])
		{
			return Arrays.stream((Object[]) message)
				.map(ClientMessages::text)
				.filter(s -> !s.isEmpty())
				.collect(Collectors.joining(" "));
		}
		return String.valueOf(message).trim();
	}

	private static List<JDialog> dialogs()
	{
		List<JDialog> out = new ArrayList<>();
		for (Window w : Window.getWindows())
		{
			if (w instanceof JDialog && w.isShowing() && ((JDialog) w).isModal() && pane((JDialog) w) != null)
			{
				out.add((JDialog) w);
			}
		}
		return out;
	}

	private static List<String> texts(List<JDialog> dialogs)
	{
		return dialogs.stream().map(d -> text(pane(d).getMessage())).collect(Collectors.toList());
	}

	private static JOptionPane pane(Container c)
	{
		for (Component child : c.getComponents())
		{
			if (child instanceof JOptionPane)
			{
				return (JOptionPane) child;
			}
			if (child instanceof Container)
			{
				JOptionPane p = pane((Container) child);
				if (p != null)
				{
					return p;
				}
			}
		}
		return null;
	}

	/** Runs on Swing's thread: a modal window's own loop keeps serving it. */
	private static List<String> onEdt(Supplier<List<String>> work)
	{
		if (SwingUtilities.isEventDispatchThread())
		{
			return work.get();
		}
		AtomicReference<List<String>> out = new AtomicReference<>(Collections.emptyList());
		try
		{
			SwingUtilities.invokeAndWait(() -> out.set(work.get()));
		}
		catch (InterruptedException e)
		{
			Thread.currentThread().interrupt();
		}
		catch (Exception e)
		{
			log.warn("client messages: {}", e.getMessage());
		}
		return out.get();
	}
}
