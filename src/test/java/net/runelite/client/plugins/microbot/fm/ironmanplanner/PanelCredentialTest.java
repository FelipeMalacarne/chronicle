package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import okhttp3.Request;
import org.junit.jupiter.api.Test;

class PanelCredentialTest
{
	private static final String INSTALLATION = "3f2b8c1e-9a4d-4e7b-8c2a-1d5e6f7a8b9c";

	private static Request applied(PlannerCredential c)
	{
		Request.Builder b = new Request.Builder().url("http://127.0.0.1:8080/api/ingest");
		c.apply(b);
		return b.build();
	}

	@Test
	void noCredentialSendsTheInstallationAndNoToken()
	{
		Request r = applied(new NoCredential(INSTALLATION, ""));
		assertNull(r.header("Authorization"));
		assertEquals(INSTALLATION, r.header(NoCredential.HEADER_INSTALLATION));
		assertNull(r.header(NoCredential.HEADER_ACCOUNT), "no Conta, no header");
		assertTrue(new NoCredential(INSTALLATION, "").ready());
		assertFalse(new NoCredential("", "").ready(), "never without an installation id");

		Request withAccount = applied(new NoCredential(INSTALLATION, " acc_main "));
		assertEquals("acc_main", withAccount.header(NoCredential.HEADER_ACCOUNT));
	}

	@Test
	void tokenCredentialSendsOnlyTheBearer()
	{
		Request r = applied(new TokenCredential(" ipl_secret "));
		assertEquals("Bearer ipl_secret", r.header("Authorization"));
		assertNull(r.header(NoCredential.HEADER_INSTALLATION));
		assertFalse(new TokenCredential("").ready());
		assertFalse(new TokenCredential(null).ready());
		assertTrue(new TokenCredential("ipl_x").ready());
	}

	@Test
	void secretsStayOutOfToStringAndFingerprints()
	{
		TokenCredential t = new TokenCredential("ipl_secret");
		assertFalse(t.toString().contains("ipl_secret"));
		assertFalse(t.fingerprint().contains("ipl_secret"));
		assertNotEquals(t.fingerprint(), new TokenCredential("ipl_other").fingerprint());
		assertEquals(t.fingerprint(), new TokenCredential("ipl_secret").fingerprint());
		assertNotEquals(new NoCredential(INSTALLATION, "").fingerprint(), new NoCredential(INSTALLATION, "acc_alt").fingerprint());
	}

	@Test
	void theConfigModeSelectsTheCredential()
	{
		assertTrue(PlannerCredential.of(IronmanPlannerConfig.CredentialMode.NONE, "ipl_ignored", INSTALLATION, "") instanceof NoCredential);
		assertTrue(PlannerCredential.of(IronmanPlannerConfig.CredentialMode.TOKEN, "ipl_x", INSTALLATION, "acc") instanceof TokenCredential);
		assertTrue(PlannerCredential.of(null, "", INSTALLATION, "") instanceof NoCredential, "default is the dev credential");
	}

	@Test
	void installationIdIsGeneratedOnceAndKept()
	{
		List<String> saved = new ArrayList<>();
		UUID fixed = UUID.fromString(INSTALLATION);
		String first = InstallationId.ensure("", saved::add, () -> fixed);
		assertEquals(INSTALLATION, first);
		assertEquals(List.of(INSTALLATION), saved);

		// the next start reads it back from the config: nothing generated, nothing saved
		String again = InstallationId.ensure(saved.get(0), saved::add, () -> { throw new AssertionError("regenerated"); });
		assertEquals(first, again);
		assertEquals(1, saved.size());

		String repaired = InstallationId.ensure("not-a-uuid", saved::add, UUID::randomUUID);
		assertTrue(InstallationId.isValid(repaired));
		assertEquals(repaired, saved.get(1), "a broken value is replaced and saved");
		assertTrue(InstallationId.isValid(InstallationId.ensure(null, saved::add, UUID::randomUUID)));
		assertFalse(InstallationId.isValid(INSTALLATION.toUpperCase()));
	}
}
