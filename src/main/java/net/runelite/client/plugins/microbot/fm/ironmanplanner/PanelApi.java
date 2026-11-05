package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import okhttp3.HttpUrl;

/** The planner calls behind the route panel; blocking, called only off the client and Swing threads. */
interface PanelApi
{
	/** GET /api/plugin/route?limit=. */
	PlannerHttpClient.Result<RouteWindow> fetchRoute(HttpUrl base, PlannerCredential credential, int limit);

	/** PATCH /api/plugin/guides/{guide}/steps/{id}/mark with {kind}; answers with the new window. */
	PlannerHttpClient.Result<RouteWindow> mark(HttpUrl base, PlannerCredential credential, String guide, String stepId, String kind, int limit);

	/** POST /api/plugin/guides/{guide}/marks with {restore:[{step_id, mark}]}; answers with the new window. */
	PlannerHttpClient.Result<RouteWindow> restore(HttpUrl base, PlannerCredential credential, String guide, String stepId, RouteWindow.Mark mark, int limit);
}
