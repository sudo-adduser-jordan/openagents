import { useEffect, useState } from "react";
import openAgentsLogo from "../../../assets/open-agents-logo.svg";
import { openAgentsBridge } from "../lib/bridge";
import { useSystemRequirementsGate } from "../hooks/useSystemRequirementsGate";
import { InstallDependencyDialog } from "./InstallDependencyDialog";

const STARTUP_PHRASES = [
	"Starting local services",
	"Connecting to the daemon",
	"Loading workspaces",
	"Preparing your board",
] as const;

// Shown instead of the normal phrases when the current boot is a post-update
// relaunch, so the swap reads as "the app is updating" rather than "the app is
// slow to connect".
const UPDATE_PHRASES = [
	"Updating Open Agents",
	"Restarting Open Agents",
	"Starting local services",
	"Preparing your board",
] as const;

const PHRASE_INTERVAL_MS = 2_200;

export function DaemonStartupLoader() {
	const [phraseIndex, setPhraseIndex] = useState(0);
	const [postUpdate, setPostUpdate] = useState(false);
	const {
		query: requirementsQuery,
		requirements,
		requirementsBlocked,
	} = useSystemRequirementsGate();

	useEffect(() => {
		let active = true;
		// Defensive: the loader must render even when the updates bridge is absent
		// (web fallback, or a test/preload stub without this namespace). A missing
		// signal simply means "not a post-update relaunch".
		const isPostUpdateRelaunch = openAgentsBridge.updates?.isPostUpdateRelaunch;
		if (typeof isPostUpdateRelaunch !== "function") {
			return;
		}
		void isPostUpdateRelaunch().then(
			(value) => {
				if (active) setPostUpdate(value);
			},
			() => undefined,
		);
		return () => {
			active = false;
		};
	}, []);

	const phrases = postUpdate ? UPDATE_PHRASES : STARTUP_PHRASES;

	useEffect(() => {
		const timer = window.setInterval(() => {
			setPhraseIndex((current) => (current + 1) % phrases.length);
		}, PHRASE_INTERVAL_MS);
		return () => window.clearInterval(timer);
	}, [phrases.length]);

	const phrase = phrases[phraseIndex % phrases.length];

	return (
		<div
			aria-busy="true"
			aria-label={`${"Open Agents"} is starting`}
			aria-live="polite"
			className="open-agents-startup-screen flex h-full w-full items-center justify-center bg-background text-foreground"
			data-testid="daemon-startup-loader"
			role="status"
		>
			<div className="open-agents-startup-content flex -translate-y-[3vh] flex-col items-center text-center">
				<div className="grid h-28 w-32 place-items-center" aria-hidden="true">
					<img className="open-agents-startup-logo h-22 w-25 object-contain" src={openAgentsLogo} alt="" />
				</div>
				<p className="mt-5 text-base font-semibold tracking-tight text-foreground">Open Agents</p>
				<p className="mt-2 min-h-5 text-md-sm text-muted-foreground">
					<span aria-hidden="true" className={phraseIndex === 0 ? undefined : "open-agents-startup-status"} key={phraseIndex}>
						{phrase}
					</span>
				</p>
				<div className="open-agents-startup-dots mt-3 flex h-4 items-center gap-1.5" aria-hidden="true">
					<span />
					<span />
					<span />
				</div>
			</div>
			{requirementsBlocked ? (
				<InstallDependencyDialog requirements={requirements} onRefetchRequirements={() => requirementsQuery.refetch()} />
			) : null}
		</div>
	);
}
