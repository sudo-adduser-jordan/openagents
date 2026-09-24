import { Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useBrowserDownloads } from "../../hooks/useBrowserDownloads";
import { BrowserDownloadsList } from "../BrowserDownloadsList";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { SettingsSection } from "./SettingsSection";

export function BrowserDownloadsSection({ titleHidden }: { titleHidden?: boolean }) {
	const { downloads, error, action, clear } = useBrowserDownloads();
	const [query, setQuery] = useState("");
	const filtered = useMemo(() => {
		const normalized = query.trim().toLocaleLowerCase();
		return normalized ? downloads.filter((download) => download.fileName.toLocaleLowerCase().includes(normalized)) : downloads;
	}, [downloads, query]);
	const hasFinished = downloads.some((download) => download.status !== "progressing" && download.status !== "paused");

	return (
		<SettingsSection title="Downloads" sectionId="downloads" titleHidden={titleHidden}>
			<div className="flex items-center gap-2">
				<Input aria-label="Search download history" onChange={(event) => setQuery(event.target.value)} placeholder="Search download history" value={query} />
				<Button disabled={!hasFinished} onClick={() => void clear()} type="button" variant="outline">
					<Trash2 aria-hidden="true" className="size-icon-base" />
					{"Clear all"}
				</Button>
			</div>
			<div className="mt-3">
				<BrowserDownloadsList downloads={filtered} error={error} onAction={(id, nextAction) => void action(id, nextAction)} />
			</div>
		</SettingsSection>
	);
}
