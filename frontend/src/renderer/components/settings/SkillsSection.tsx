import { useParams } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";

import { apiErrorCode, apiErrorMessage } from "../../lib/api-client";
import { useConversationSkills } from "../../hooks/useConversation";
import { skillSourceLabel } from "../chat/composerSuggest";
import { SettingsSection } from "./SettingsSection";

/**
 * Skills: the named skills the open session's provider will accept.
 *
 * The daemon only answers this for a live session, so the page is session-scoped
 * rather than a global inventory — and it deliberately reads the same
 * `conversationSkillsQueryKey` the composer's `/` menu does, so the two surfaces
 * are one cached request instead of two answers that can drift apart.
 *
 * Read-only on purpose. Open Agents does not own the files that define a skill
 * (they come from the user's agent config and the repo's own files), so there is
 * nothing here to save.
 *
 * Requires a router above it: the active session is route state, and the only
 * mount of this page is the shell's settings dialog, which is inside the router.
 */
export function SkillsSection({ titleHidden }: { titleHidden?: boolean }) {
	// The same route-derived selection the sidebar and topbar use, so the panel
	// always describes the session the user is actually looking at. Settings can
	// be opened with nothing selected — from the board, the home page, or a
	// project route — and that is a real state, not a failed lookup: the catalog
	// is per-session, so there is nothing to list until a session is open.
	const params = useParams({ strict: false }) as { sessionId?: string };
	const sessionId = params.sessionId;
	// Called unconditionally because it is a hook; `enabled` carries the "no
	// session" decision so the shared query is never keyed on an empty session id
	// and the composer is left to own the real request.
	const { skills, isLoading, error } = useConversationSkills(sessionId, Boolean(sessionId));

	// A failed read is not an empty catalog. The daemon answers 409
	// CHAT_CONTROLLER_NOT_READY until a live controller owns the session, which is
	// the normal state of a session that exists but has not started its agent --
	// reporting "no skills" there would state an absence the provider never
	// confirmed. A refetch that fails after a good read keeps the catalog it had
	// and only adds a note, rather than blanking a list mid-read.
	const failed = error != null;
	const waitingForController = apiErrorCode(error) === "CHAT_CONTROLLER_NOT_READY";
	const unreadable = failed && skills.length === 0;

	return (
		<SettingsSection title="Skills" sectionId="skills" titleHidden={titleHidden} grouped>
			<p className="text-xs leading-relaxed text-muted-foreground">
				{"These are the slash commands the open session's agent accepts. They come from the agent's own configuration and from this repository's files, and Open Agents does not edit either."}
			</p>
			{!sessionId ? (
				<Notice
					title="No session is open"
					body="Open a session and its skills are listed here, the same list its composer offers after a slash."
				/>
			) : unreadable ? (
				waitingForController ? (
					<Notice
						title="This session's agent isn't running yet"
						body="Skills are published once the agent starts. Start the session and the list appears here."
					/>
				) : (
					<p className="px-3 py-3 text-xs text-error" role="alert">
						{apiErrorMessage(error, "Could not read this session's skills")}
					</p>
				)
			) : isLoading ? (
				<p className="flex items-center gap-2 px-3 py-3 text-xs text-muted-foreground" role="status">
					<Loader2 className="size-icon-sm shrink-0 animate-spin motion-reduce:animate-none" aria-hidden="true" />
					{"Loading skills…"}
				</p>
			) : skills.length === 0 ? (
				<Notice
					title="This session has no skills"
					body="The agent reported an empty catalog, so typing a slash in the composer opens no menu."
				/>
			) : (
				<>
					{skills.map((skill) => {
						// The composer's slash menu labels skills through this same
						// function, so a skill is never described two ways.
						const source = skillSourceLabel(skill.source);
						return (
							<div className="settings-row-bar h-auto min-h-(--size-settings-row) items-start py-3" key={skill.name}>
								<div className="min-w-0 flex-1">
									<p className="flex min-w-0 flex-wrap items-baseline gap-2 text-sm leading-5 text-settings-label">
										<span className="font-mono">{`/${skill.displayName || skill.name}`}</span>
										{source ? <span className="text-xs text-muted-foreground">{source}</span> : null}
									</p>
									{skill.description ? (
										<p className="mt-0.5 text-xs leading-4 text-muted-foreground">{skill.description}</p>
									) : null}
									{skill.inputHint ? (
										<p className="mt-0.5 font-mono text-xs leading-4 text-muted-foreground">{skill.inputHint}</p>
									) : null}
								</div>
							</div>
						);
					})}
					{failed ? (
						<p className="px-3 py-3 text-xs text-error" role="alert">
							{apiErrorMessage(error, "Could not refresh this session's skills")}
						</p>
					) : null}
				</>
			)}
		</SettingsSection>
	);
}

function Notice({ title, body }: { title: string; body: string }) {
	return (
		<div className="px-3 py-3">
			<p className="text-sm leading-5 text-settings-label">{title}</p>
			<p className="mt-0.5 text-xs leading-4 text-muted-foreground">{body}</p>
		</div>
	);
}
