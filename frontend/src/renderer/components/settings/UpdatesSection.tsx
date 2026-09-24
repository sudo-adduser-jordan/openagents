import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { AlertTriangle, CheckCircle2, Clock3, Download, Info, Loader2, RefreshCw } from "lucide-react";
import { aoBridge } from "../../lib/bridge";
import { cn } from "../../lib/utils";
import { parseNightlyVersion } from "../../lib/build-channel";
import { useUiStore } from "../../stores/ui-store";
import { useRequestUpdateInstall } from "../../hooks/useRequestUpdateInstall";
import { useUpdateStatus, requestUpdateDownload } from "../../hooks/useUpdateStatus";
import type { UpdateChannel, UpdateSettings, UpdateState, UpdateStatus } from "../../../main/update-settings";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Switch } from "../ui/switch";
import { ConfirmDialog } from "../ConfirmDialog";
import { SettingsOptionMenu } from "./SettingsOptionMenu";
import { SettingsRow } from "./SettingsRow";
import { SettingsSection } from "./SettingsSection";

export const updateSettingsQueryKey = ["update-settings"] as const;

type PrimaryValue = UpdateChannel | "feature";

const DEFAULT_SETTINGS: UpdateSettings = { enabled: false, channel: "latest", nightlyAck: false, feature: null };
const MIN_MANUAL_CHECK_VISIBLE_MS = 1_000;
// Hard ceiling on how long a manual check may hold the button. The IPC call is
// awaited and normally settles on its own, but if it ever does not, the pending
// request id disables the Check button for the rest of the session with nothing
// on screen to explain it. Releasing the button is always safe: the main process
// serializes updater operations, so a redundant check queues rather than racing.
const MAX_MANUAL_CHECK_MS = 90_000;

let updateRequestSequence = 0;

function nextUpdateRequestId(prefix = "feature-update"): string {
	updateRequestSequence += 1;
	return `${prefix}-${updateRequestSequence}`;
}

export function UpdatesSection({ titleHidden }: { titleHidden?: boolean } = {}) {
	const queryClient = useQueryClient();
	const query = useQuery({
		queryKey: updateSettingsQueryKey,
		queryFn: () => aoBridge.updateSettings.get(),
		refetchInterval: 3_000,
	});

	const [form, setForm] = useState<UpdateSettings>(DEFAULT_SETTINGS);
	const formRef = useRef(form);
	formRef.current = form;
	const [showFeature, setShowFeature] = useState(false);
	const [savingField, setSavingField] = useState<"automatic" | "channel" | null>(null);
	const [pendingPin, setPendingPin] = useState<{ pr: number; title: string } | null>(null);
	const [manualCheckFailure, setManualCheckFailure] = useState<string | null>(null);
	const [manualCheckRequestId, setManualCheckRequestId] = useState<string | null>(null);
	const [channelSwitch, setChannelSwitch] = useState<{ channel: UpdateChannel; requestId: string } | null>(null);
	const channelSwitchRef = useRef<typeof channelSwitch>(null);
	channelSwitchRef.current = channelSwitch;
	const manualCheckStartedAtRef = useRef<number | null>(null);
	const manualCheckFinishTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const manualCheckWatchdogRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const developerMode = useUiStore((state) => state.developerMode);

	const clearManualCheckWatchdog = () => {
		if (manualCheckWatchdogRef.current === null) return;
		clearTimeout(manualCheckWatchdogRef.current);
		manualCheckWatchdogRef.current = null;
	};

	const finishManualCheck = (requestId: string, error?: unknown) => {
		if (error) setManualCheckFailure(error instanceof Error ? error.message : "Update failed.");
		clearManualCheckWatchdog();
		if (manualCheckFinishTimerRef.current !== null) clearTimeout(manualCheckFinishTimerRef.current);
		const elapsed = manualCheckStartedAtRef.current === null ? MIN_MANUAL_CHECK_VISIBLE_MS : Date.now() - manualCheckStartedAtRef.current;
		const clear = () => {
			manualCheckFinishTimerRef.current = null;
			setManualCheckRequestId((pending) => {
				if (pending !== requestId) return pending;
				manualCheckStartedAtRef.current = null;
				return null;
			});
		};
		const remaining = Math.max(0, MIN_MANUAL_CHECK_VISIBLE_MS - elapsed);
		if (remaining === 0) clear();
		else manualCheckFinishTimerRef.current = setTimeout(clear, remaining);
	};

	const startManualCheck = (requestId: string) => {
		setManualCheckFailure(null);
		clearManualCheckWatchdog();
		manualCheckStartedAtRef.current = Date.now();
		setManualCheckRequestId(requestId);
		manualCheckWatchdogRef.current = setTimeout(() => {
			manualCheckWatchdogRef.current = null;
			setManualCheckFailure("Update check stopped responding. Try again.");
			setManualCheckRequestId((pending) => (pending === requestId ? null : pending));
		}, MAX_MANUAL_CHECK_MS);
	};

	const status = useUpdateStatus((next) => {
		if (next.requestId && next.requestId === manualCheckRequestId && next.state !== "checking") {
			finishManualCheck(next.requestId);
		}
		const pending = channelSwitchRef.current;
		if (pending && next.requestId === pending.requestId && ["not-available", "error", "unsupported"].includes(next.state)) {
			setChannelSwitch(null);
		}
	}, true);

	useEffect(
		() => () => {
			if (manualCheckFinishTimerRef.current !== null) clearTimeout(manualCheckFinishTimerRef.current);
			if (manualCheckWatchdogRef.current !== null) clearTimeout(manualCheckWatchdogRef.current);
		},
		[],
	);
	// Set only for the owned pin/home transition request, so unrelated hourly
	// updater events cannot auto-progress through download/install.
	const autoProgressRef = useRef<string | null>(null);
	const handledStatusRef = useRef<UpdateState | null>(null);

	useEffect(() => {
		if (query.data) setForm(query.data);
	}, [query.data]);

	useEffect(() => {
		const requestId = autoProgressRef.current;
		if (!requestId || status.requestId !== requestId) return;
		if (handledStatusRef.current === status.state) return;
		handledStatusRef.current = status.state;
		if (status.state === "available") {
			void requestUpdateDownload(requestId);
		} else if (status.state === "downloaded") {
			void aoBridge.updates.install();
			autoProgressRef.current = null;
		} else if (status.state === "error" || status.state === "unsupported" || status.state === "not-available") {
			autoProgressRef.current = null;
		}
	}, [status]);

	const save = useMutation({
		mutationFn: async (next: UpdateSettings) => {
			await aoBridge.updateSettings.set(next);
			return next;
		},
		onSuccess: (next) => {
			setSavingField(null);
			setForm(next);
			void queryClient.invalidateQueries({ queryKey: updateSettingsQueryKey });
		},
		onError: () => {
			setSavingField(null);
			const previous = queryClient.getQueryData<UpdateSettings>(updateSettingsQueryKey);
			if (previous) setForm(previous);
		},
	});

	const channelOptions: { value: PrimaryValue; label: string }[] = [
		{ value: "latest", label: "Stable" },
		{ value: "nightly", label: "Nightly (Pre-release)" },
		{ value: "feature", label: "Feature Releases" },
	];
	const primaryValue: PrimaryValue = developerMode && (form.feature !== null || showFeature) ? "feature" : form.channel;

	const setEnabled = (enabled: boolean) => {
		setSavingField("automatic");
		const next = { ...formRef.current, enabled };
		setForm(next);
		save.mutate(next);
	};

	const handlePrimaryChannel = (value: PrimaryValue) => {
		if (value === "feature") {
			setShowFeature(true);
			return;
		}
		setShowFeature(false);
		setSavingField("channel");
		const next = {
			...formRef.current,
			channel: value,
			nightlyAck: value === "nightly",
			feature: null,
		};
		setForm(next);
		save.mutate(next);
		const requestId = nextUpdateRequestId("channel-update");
		setChannelSwitch({ channel: value, requestId });
		startManualCheck(requestId);
		void aoBridge.updates
			.check({ settings: next, requestId })
			.catch(() => {
				setChannelSwitch((pending) => (pending?.requestId === requestId ? null : pending));
			})
			.finally(() => finishManualCheck(requestId));
	};

	const confirmPinBuild = async () => {
		if (!pendingPin) return;
		const { pr } = pendingPin;
		setPendingPin(null);
		const next = { ...formRef.current, feature: { pr } };
		setForm(next);
		const requestId = nextUpdateRequestId();
		autoProgressRef.current = requestId;
		handledStatusRef.current = null;
		try {
			await aoBridge.updates.check({ settings: next, requestId });
			void queryClient.invalidateQueries({ queryKey: updateSettingsQueryKey });
		} catch {
			if (autoProgressRef.current === requestId) autoProgressRef.current = null;
			void queryClient.invalidateQueries({ queryKey: updateSettingsQueryKey });
		}
	};

	const handleReturnToHome = async () => {
		setShowFeature(false);
		// Optimistic; the main process clears the pin against persisted state.
		setForm({ ...formRef.current, feature: null });
		const requestId = nextUpdateRequestId();
		autoProgressRef.current = requestId;
		handledStatusRef.current = null;
		try {
			// Single updater-serialized op: clears the pin and checks the home channel
			// atomically, so a concurrent settings-write cannot restore the pin.
			await aoBridge.updates.returnHome(requestId);
			void queryClient.invalidateQueries({ queryKey: updateSettingsQueryKey });
		} catch {
			if (autoProgressRef.current === requestId) autoProgressRef.current = null;
			// The optimistic form update may now disagree with disk; re-sync to truth.
			void queryClient.invalidateQueries({ queryKey: updateSettingsQueryKey });
		}
	};

	const activeQuery = useQuery({
		queryKey: ["feature-active"],
		queryFn: () => aoBridge.featureBuilds.getActive(),
	});
	const activeBuild = activeQuery.data ?? null;
	// Show the escape hatch whenever a feature build is running or pinned.
	const featurePr = activeBuild?.pr ?? (developerMode ? null : (form.feature?.pr ?? null));

	return (
		<>
			<SettingsSection title="Updates" sectionId="updates" titleHidden={titleHidden} grouped>
				<UpdateActions
					status={manualCheckFailure ? { ...status, state: "error", message: manualCheckFailure } : status}
					manualCheckRequestId={manualCheckRequestId}
					startManualCheck={startManualCheck}
					finishManualCheck={finishManualCheck}
					channelSwitch={channelSwitch}
				/>

				{featurePr != null && (
					<div className="settings-row-bar h-auto min-h-(--size-settings-row) items-start gap-3 py-3">
						<Badge className="mt-0.5" variant="accent">PR #{featurePr}</Badge>
						<div className="min-w-0 flex-1">
							<p className="text-sm leading-5 text-settings-label">
								{activeBuild
									? `You are on PR #${featurePr}'s build.`
									: `PR #${featurePr} is pinned but not yet installed.`}
							</p>
							<p className="mt-1 text-xs leading-4 text-settings-muted">
								{`Automatic updates, if enabled, keep tracking PR #${featurePr} until you return home or the build retires.`}
							</p>
						</div>
						<Button type="button" variant="outline" size="sm" onClick={() => void handleReturnToHome()}>
							{form.channel === "nightly" ? "Return to Nightly" : "Return to Stable"}
						</Button>
					</div>
				)}

				<SettingsRow
					label="Automatic Updates"
					description="Downloads updates in the background. They install when you quit the app."
				>
					<Switch
						aria-label="Automatic Updates"
						checked={form.enabled}
						onCheckedChange={setEnabled}
						disabled={savingField === "automatic"}
					/>
				</SettingsRow>

				<SettingsRow
					label="Updates channel"
					description="Stable is released and tested. Nightly is the newest daily build."
				>
					<SettingsOptionMenu
						aria-label="Updates channel"
						value={primaryValue}
						options={developerMode ? channelOptions : channelOptions.filter((option) => option.value !== "feature")}
						onChange={handlePrimaryChannel}
						disabled={savingField === "channel"}
					/>
				</SettingsRow>

				{primaryValue === "feature" && (
					<FeatureBuildsSelect
						currentPr={form.feature?.pr ?? null}
						onPin={(pr, title) => setPendingPin({ pr, title })}
					/>
				)}

				{primaryValue === "nightly" && (
					// Helper copy for the row above, not an alert. This is a standing
					// property of the channel the user chose, not something wrong right
					// now, and --color-warning is a red-orange one step from
					// --destructive: rendered in it, a permanent note read as a permanent
					// failure. Red is kept for states that are actually broken and
					// actionable — failing checks, a stale network stack, update errors.
					<p className="nightly-warning -mt-1 flex items-start gap-2 px-(--size-settings-row-padding) pb-(--size-settings-row-padding) text-xs leading-4 text-settings-muted">
						<Info className="mt-px size-icon-sm shrink-0" aria-hidden="true" />
						<span className="min-w-0">{"Nightly updates daily and may be unstable or cause data loss."}</span>
					</p>
				)}

				{save.isError && (
					<p className="mt-2 px-(--size-settings-row-padding) text-xs text-error">
						{save.error instanceof Error ? save.error.message : "Save failed"}
					</p>
				)}
			</SettingsSection>
			<ConfirmDialog
				open={pendingPin !== null}
				title="Switch feature build?"
				description={pendingPin ? `Switch to PR #${pendingPin.pr}: ${pendingPin.title}? The app will download the feature build and restart.` : null}
				confirmLabel="Confirm"
				onConfirm={() => void confirmPinBuild()}
				onOpenChange={(open) => !open && setPendingPin(null)}
			/>
		</>
	);
}

function FeatureBuildsSelect({
	currentPr,
	onPin,
}: {
	currentPr: number | null;
	onPin: (pr: number, title: string) => void;
}) {
	const buildsQuery = useQuery({ queryKey: ["feature-builds"], queryFn: () => aoBridge.featureBuilds.list() });
	const builds = buildsQuery.data ?? [];

	if (!buildsQuery.isLoading && builds.length === 0) {
		return <p className="px-3 text-xs text-settings-muted">{"No live feature releases."}</p>;
	}

	return (
		<SettingsRow label="Feature build">
			<SettingsOptionMenu
				aria-label="Feature build"
				value={currentPr === null ? "__none__" : currentPr.toString()}
				placeholder="Select a feature build..."
				options={builds.map((build) => ({ value: build.pr.toString(), label: `PR #${build.pr}: ${build.title}` }))}
				disabled={buildsQuery.isLoading}
				onChange={(value) => {
					const build = builds.find((item) => item.pr === Number(value));
					if (build) onPin(build.pr, build.title);
				}}
			/>
		</SettingsRow>
	);
}

function UpdateActions({
	status,
	manualCheckRequestId,
	startManualCheck,
	finishManualCheck,
	channelSwitch,
}: {
	status: UpdateStatus;
	manualCheckRequestId: string | null;
	startManualCheck: (requestId: string) => void;
	finishManualCheck: (requestId: string, error?: unknown) => void;
	channelSwitch: { channel: UpdateChannel; requestId: string } | null;
}) {
	const version = useQuery({ queryKey: ["app-version"], queryFn: () => aoBridge.app.getVersion() });
	const requestUpdateInstall = useRequestUpdateInstall();
	const installedChannel = installedUpdateChannel(version.data);
	const installed = parseNightlyVersion(version.data);

	const manualCheckPending = manualCheckRequestId !== null;
	const checking = status.state === "checking" || manualCheckPending;
	const downloading = status.state === "downloading" || status.state === "preparing";
	// Only the user's OWN in-flight check blocks the button. A background check
	// also reports "checking", and gating on that swallowed the first click
	// whenever Settings was opened during one — every 15 minutes on nightly.
	// A click during a background check is fine: the main process serializes
	// updater operations, so the manual check simply queues behind it.
	const busy = manualCheckPending || downloading;
	// The minimum-spinner window keeps "checking" on screen briefly after the
	// updater has already answered. The button is the single visible in-progress
	// indicator; the status row resumes with the updater's terminal result.
	const displayState: UpdateState = checking && !downloading && status.state !== "error" && status.state !== "downloaded" ? "checking" : status.state;
	const showStatusState = displayState !== "checking";

	const [now, setNow] = useState(Date.now());
	useEffect(() => {
		const timer = setInterval(() => setNow(Date.now()), 1_000);
		return () => clearInterval(timer);
	}, []);
	const checkedAt = status.checkedAt
		? new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "medium" }).format(status.checkedAt)
		: null;

	const channelSwitchInFlight = channelSwitch !== null && (!status.requestId || status.requestId === channelSwitch.requestId);
	// Use the live updater state, not displayState: the manual-check minimum
	// spinner time forces displayState back to "checking" even after a channel
	// switch finds an update, which would hide this guidance until the timer fires.
	// Not shown for "downloaded": the status line already says "Downloaded.
	// Restart to finish updating." and names the build under it, so adding
	// "Restart to switch to Nightly." put two restart sentences side by side.
	const channelSwitchMessage = channelSwitchInFlight &&
		(status.state === "available" || status.state === "downloading")
		? `Update and restart to switch to ${channelSwitch.channel === "nightly" ? "Nightly" : "Stable"}.`
		: null;

	const checkNow = async () => {
		const requestId = nextUpdateRequestId("manual-update");
		startManualCheck(requestId);
		try {
			await aoBridge.updates.check({ requestId });
		} catch (error) {
			finishManualCheck(requestId, error);
		} finally {
			finishManualCheck(requestId);
		}
	};

	// Deliberately no border or accent fill on this block. --primary is
	// oklch(0.92 0.004 286.32) in dark and near-black in light: both are
	// effectively neutral, so tinting drew a grey frame that carried no meaning
	// and competed with the row backgrounds around it. The primary button and
	// the status line already carry the emphasis.
	return (
		<div className="settings-row-bar update-status-row h-auto flex-col items-stretch gap-4 py-4">
			<div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
				{/* Identity. The nightly stamp lives on its own monospace line: as one
				    heading it wrapped mid-token and swallowed the row. */}
				<div className="min-w-0 flex-1">
					<p className="text-caption font-medium uppercase tracking-wide text-settings-muted">
						{"Installed version"}
					</p>
					<div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
						<span
							aria-label={`Current version - ${version.data ? `v${version.data}` : "…"}`}
							className="text-2xl font-semibold leading-none tracking-tight tabular-nums text-settings-label"
							data-testid="app-version"
						>
							{version.data ? `v${installed?.base ?? version.data}` : "…"}
						</span>
						<Badge data-testid="installed-update-channel" variant="neutral">
							{installedChannel === "nightly" ? "Nightly" : "Stable"}
						</Badge>
					</div>
					{installed && (
						// Two spans, not one string: the version needs break-all so a long
						// nightly stamp can never overflow, but break-all applied to the
						// whole line also snapped the date mid-word ("20/26, 5:21 PM").
						<p className="mt-1.5 flex min-w-0 flex-wrap items-baseline gap-x-1.5 text-caption leading-4 text-settings-muted">
							<span className="min-w-0 break-all font-mono">{version.data}</span>
							<span className="whitespace-nowrap">
								{`Built ${new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(installed.builtAt)}`}
							</span>
						</p>
					)}
				</div>

				{/* Actions. Fixed to the right and never widened by a version string:
				    the primary label used to carry the full nightly stamp and grew
				    across the heading. The target build is named in the status line. */}
				<div className="flex w-full shrink-0 flex-wrap items-center gap-2 sm:w-auto sm:justify-end [&>button]:flex-1 sm:[&>button]:flex-none">
					{status.state === "available" && (
						<Button
							type="button"
							variant="primary"
							size="sm"
							aria-label={status.version ? `Update to ${`v${status.version}`}` : "Update to latest"}
							onClick={() => void requestUpdateDownload()}
						>
							<Download className="size-icon-sm" aria-hidden="true" />
							{"Update"}
						</Button>
					)}
					{(!downloading && (status.state === "downloaded" || (status.staged && status.staged.ready !== false))) && (
						// Opens the restart confirmation rather than installing outright:
						// installing quits the app, which costs a turn on any chat session
						// running a daemon-owned driver.
						<Button type="button" variant="primary" size="sm" onClick={requestUpdateInstall}>
							<RefreshCw className="size-icon-sm" aria-hidden="true" />
							{"Restart & install"}
						</Button>
					)}
					{/* Always rendered. Hiding it whenever something was available or
					    staged left no way to re-check at all: a user sitting on a build
					    that will not install had a single dead Restart button and no
					    other action on the page. */}
					<Button
						type="button"
						aria-label={checking ? "Checking for updates…" : "Check for updates"}
						aria-describedby={showStatusState ? "update-status-line" : undefined}
						variant="outline"
						size="sm"
						onClick={() => void checkNow()}
						disabled={busy}
					>
						{checking ? (
							<Loader2 className="size-icon-sm animate-spin motion-reduce:animate-none" aria-hidden="true" />
						) : (
							<RefreshCw className="size-icon-sm" aria-hidden="true" />
						)}
						{checking ? "Checking for updates…" : "Check for updates"}
					</Button>
				</div>
			</div>

			{/* One status block, in normal flow. It used to be an absolutely
			    positioned h-5 strip, so a two-line message overlapped the
			    last-checked line and the cross-fade left it looking half-rendered. */}
			<div
				id="update-status-line"
				data-testid="update-status-line"
				role="status"
				aria-live="polite"
				aria-atomic="true"
				aria-busy={checking}
				// Keep the status slot's height while the button owns the checking
				// feedback, so notices and last-checked metadata below do not jump up.
				className="flex min-h-5 min-w-0 flex-col gap-1"
			>
				{showStatusState && <UpdateStatusLine state={displayState} status={status} />}
				{status.state === "downloading" && status.percent !== undefined && <progress aria-label="Download progress" max={100} value={status.percent} className="h-1 w-full" />}
				{status.transferred !== undefined && status.total !== undefined && <p className="text-xs tabular-nums text-settings-muted">{`${(status.transferred / 1_000_000).toFixed(1)} / ${(status.total / 1_000_000).toFixed(1)} MB`}</p>}
				{channelSwitchMessage && <p className="text-xs leading-4 text-settings-muted">{channelSwitchMessage}</p>}
				{checkedAt ? (
					<p className="flex min-w-0 items-start gap-2 text-xs leading-4 tabular-nums text-settings-muted" data-testid="update-checked-at">
						<span className="flex h-4 shrink-0 items-center">
							<Clock3 className="size-icon-sm shrink-0" aria-hidden="true" />
						</span>
						<span title={new Date(status.checkedAt!).toISOString()}>{now - status.checkedAt! < 60_000 ? "Checked just now" : `Checked ${Math.floor((now - status.checkedAt!) / 60_000)} minutes ago`} · {checkedAt}</span>
					</p>
				) : null}
			</div>

			{/* Release notes used to live only in the restart confirmation, so
			    skipping that dialog when nothing is at risk would have hidden them
			    entirely. The panel has room the dialog never did. Plain text on
			    purpose: these are the remote release body, sanitized in the main
			    process, and nothing here injects markup. */}
			{(status.state === "downloaded" || status.staged) && status.releaseNotes ? (
				<div className="mt-3" data-testid="update-release-notes">
					<p className="text-caption font-medium uppercase tracking-wide text-settings-muted">
						{"What's new"}
					</p>
					<p className="mt-1.5 max-h-40 overflow-y-auto whitespace-pre-line text-pretty text-sm leading-5 text-settings-label">
						{status.releaseNotes}
					</p>
				</div>
			) : null}

			{/* The primary error already explains the failed check. Keep a separate
			    notice only when it adds information (for example beside a staged build). */}
			{status.checkError && !(displayState === "error" && status.message === status.checkError) && (
				<UpdateNotice tone="error" text={status.checkError} />
			)}

			{!status.checkError && !status.staleCheckNudge && status.checksFailing && (
				<UpdateNotice tone="warning" text="Update checks keep failing. The app can't tell whether a newer version is out." />
			)}

			{status.staleCheckNudge && <UpdateNotice tone="warning" text="Updates haven't been able to check for a while — restarting the app usually fixes this." />}
		</div>
	);
}

/** Contained caution/failure note. A bare coloured paragraph read as body copy. */
function UpdateNotice({ tone, text }: { tone: "warning" | "error"; text: string }) {
	return (
		<p
			className={cn(
				"flex items-start gap-2 rounded-md border px-3 py-2 text-xs leading-5",
				tone === "warning" ? "border-warning/30 bg-warning/8 text-warning" : "border-error/30 bg-error/8 text-error",
			)}
		>
			<AlertTriangle className="mt-0.5 size-icon-sm shrink-0" aria-hidden="true" />
			<span className="min-w-0">{text}</span>
		</p>
	);
}

function DownloadProgressIcon({ percent }: { percent: number }) {
	const clamped = Math.min(100, Math.max(0, percent));
	return (
		<span className="relative grid size-icon-sm shrink-0 place-items-center" aria-hidden="true">
			<svg className="absolute inset-0 size-full -rotate-90" viewBox="0 0 24 24" fill="none">
				<circle cx="12" cy="12" r="9" className="stroke-current/20" strokeWidth="2.5" />
				<circle cx="12" cy="12" r="9" className="stroke-current" strokeWidth="2.5" strokeLinecap="round" strokeDasharray={`${clamped * 0.5655} 56.55`} />
			</svg>
		</span>
	);
}

function installedUpdateChannel(version: string | undefined): UpdateChannel {
	return /-nightly(?:[.+]|$)/.test(version ?? "") ? "nightly" : "latest";
}

/**
 * The single status line for the section. Every state gets a visible line with
 * a leading state icon: `available` used to be suppressed entirely (the button
 * was the only clue) and `checking` was rendered `sr-only`, which meant a
 * check in progress and a wedged check looked identical — an empty panel.
 */
function UpdateStatusLine({
	state,
	status,
}: {
	state: UpdateState;
	status: UpdateStatus;
}) {
	let className = "text-settings-muted";
	let icon: ReactNode = null;
	let label: string;
	let detail: string | null = null;


	switch (state) {
		case "checking":
			icon = <Loader2 className="size-icon-sm shrink-0 animate-spin motion-reduce:animate-none" aria-hidden="true" />;
			label = "Checking for updates…";
			break;
		case "available":
			className = "text-settings-label";
			icon = <Download className="size-icon-sm shrink-0" aria-hidden="true" />;
			label = "Update available";
			detail = status.version ? `Updating to v${status.version}` : null;
			break;
		case "downloading":
			className = "text-settings-label tabular-nums";
			icon = <DownloadProgressIcon percent={status.percent ?? 0} />;
			label = status.percent === undefined ? "Starting download…" : `Downloading… ${status.percent}%`;
			detail = status.version ? `Updating to v${status.version}` : null;
			break;
		case "preparing":
			icon = <Loader2 className="size-icon-sm animate-spin" aria-hidden="true" />;
			label = "Preparing update…";
			detail = status.version ? `Updating to v${status.version}` : null;
			break;
		case "downloaded":
			className = "text-success";
			icon = <CheckCircle2 className="size-icon-sm shrink-0" aria-hidden="true" />;
			label = "Downloaded. Restart to finish updating.";
			detail = status.version ? `Updating to v${status.version}` : null;
			break;
		case "not-available":
			className = "text-success";
			icon = <CheckCircle2 className="size-icon-sm shrink-0" aria-hidden="true" />;
			label = "You're on the latest version.";
			break;
		case "unsupported":
			icon = <Info className="size-icon-sm shrink-0" aria-hidden="true" />;
			label = status.message ?? "Updates need the installed app.";
			break;
		case "error":
			className = "text-error";
			icon = <AlertTriangle className="size-icon-sm shrink-0" aria-hidden="true" />;
			label = status.netError
				? "Couldn't reach the update server — the app's network connection appears stuck. Restarting the app usually fixes this."
				: status.message ?? "Update failed.";
			break;
		default:
			label = "Updates haven't been checked yet.";
	}

	return (
		<div className={cn("flex min-w-0 items-start gap-2", className)}>
			{icon !== null && <span className="flex h-5 shrink-0 items-center">{icon}</span>}
			<div className="min-w-0">
				<p className="text-pretty text-sm font-medium leading-5">{label}</p>
				{detail !== null && (
					<p className="mt-0.5 truncate text-xs leading-4 font-normal text-settings-muted">{detail}</p>
				)}
			</div>
		</div>
	);
}
