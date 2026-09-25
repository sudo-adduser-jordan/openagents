import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { apiClient, apiErrorMessage } from "../../lib/api-client";
import { Button } from "../ui/button";
import { SettingsRow } from "./SettingsRow";
import { SettingsSection } from "./SettingsSection";

/**
 * Tools: the user's own opencode configuration, and the tool policy Open Agents
 * layers on top of it.
 *
 * This edits a file opencode owns, not Open Agents state, so two rules hold
 * throughout. Nothing here is saved except by an explicit click, and the daemon
 * writes the text back verbatim -- re-serializing a hand-edited config would
 * delete every comment in it.
 */
export function OpencodeConfigSection({ titleHidden }: { titleHidden?: boolean }) {
	const queryClient = useQueryClient();
	const queryKey = ["opencode-config"] as const;

	const config = useQuery({
		queryKey,
		queryFn: async () => {
			const { data, error } = await apiClient.GET("/api/v1/settings/opencode-config");
			if (error) throw new Error(apiErrorMessage(error, "Could not read the opencode config"));
			return data;
		},
	});

	// Undefined means "the user has not edited", so the editor simply follows the
	// served text until they type. An earlier version mirrored the server value
	// into a draft with an effect, which latched the pre-load empty string and
	// then refused to adopt the file.
	const [draft, setDraft] = useState<string | undefined>(undefined);
	const [saved, setSaved] = useState(false);

	const served = config.data?.content ?? "";

	const save = useMutation({
		mutationFn: async (content: string) => {
			const { error } = await apiClient.PUT("/api/v1/settings/opencode-config", {
				body: { content },
			});
			if (error) throw new Error(apiErrorMessage(error, "Could not save the opencode config"));
		},
		onSuccess: async () => {
			// Drop the draft so the editor follows the file again, which also
			// re-reads anything opencode or the user changed since.
			setDraft(undefined);
			setSaved(true);
			await queryClient.invalidateQueries({ queryKey });
		},
	});

	const content = draft ?? served;
	const dirty = draft !== undefined && draft !== served;
	const warning = config.data?.warning;

	return (
		<SettingsSection titleHidden={titleHidden} title="OpenCode configuration" sectionId="opencode-config">
			<p className="text-caption text-settings-muted">
				{config.data?.path ?? "~/.config/opencode/opencode.jsonc"}
			</p>
			{warning ? (
				<p className="text-caption text-destructive" role="alert">
					{warning}
				</p>
			) : null}
			<textarea
				aria-label="OpenCode configuration"
				// Not editable until the file is in hand: typing into an empty box
				// that the load then fills reads as losing the user's text.
				disabled={config.isPending}
				className="min-h-72 w-full resize-y rounded-md border border-settings-menu bg-settings-menu px-3 py-2 font-mono text-xs leading-5 text-settings-title outline-none focus-visible:ring-2 focus-visible:ring-ring/60 disabled:opacity-60"
				onChange={(event) => {
					setDraft(event.target.value);
					setSaved(false);
				}}
				spellCheck={false}
				value={content}
			/>
			<SettingsRow label="Save">
				<Button
					disabled={!dirty || save.isPending}
					onClick={() => save.mutate(content)}
					size="sm"
					variant="outline"
				>
					{save.isPending ? "Saving…" : "Save"}
				</Button>
			</SettingsRow>
			{save.error ? (
				<p className="text-caption text-destructive" role="alert">
					{apiErrorMessage(save.error, "Could not save the opencode config")}
				</p>
			) : null}
			{saved && !dirty && !save.error ? (
				<p className="text-caption text-settings-muted" role="status">
					{"Saved. The previous contents are kept as opencode.jsonc.open-agents.bak."}
				</p>
			) : null}
			<p className="text-caption text-settings-muted">
				{
					"Open Agents never rewrites this file on its own. It is saved only when you ask, and the text is written exactly as shown above, so comments and formatting survive."
				}
			</p>
		</SettingsSection>
	);
}
