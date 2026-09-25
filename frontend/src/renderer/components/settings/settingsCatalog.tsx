import { Bot, CircleHelp, Globe2, Keyboard, RefreshCw, Settings2, Smartphone, Wrench, type LucideIcon } from "lucide-react";
import { lazy, type ReactNode } from "react";
import type { GlobalSettingsSection } from "../../stores/ui-store";
import { BrowserDownloadsSection } from "./BrowserDownloadsSection";
import { BrowserProfilesSection } from "./BrowserProfilesSection";
import { GeneralSettingsSection } from "./GeneralSettingsSection";
import { HarnessSettingsSection } from "./HarnessSettingsSection";
import { KeyboardShortcutsContent } from "./KeyboardShortcutsContent";
import { OpencodeConfigSection } from "./OpencodeConfigSection";
import { ReportProblemContent } from "./ReportProblemContent";
import { SettingsSection } from "./SettingsSection";

const UpdatesSection = lazy(async () => {
	const module = await import("./UpdatesSection");
	return { default: module.UpdatesSection };
});

export type SettingsCatalogItem = {
	id: GlobalSettingsSection;
	icon: LucideIcon;
	label: () => string;
	/** Rendered in the settings nav but greyed out and non-interactive. */
	disabled?: boolean;
	render: (titleHidden: boolean) => ReactNode;
};

function SettingsContentPanel({ children }: { children: ReactNode }) {
	return <div className="rounded-md bg-[var(--color-bg-settings-row)]">{children}</div>;
}

const globalSettingsCatalog: SettingsCatalogItem[] = [
	{
		id: "general",
		icon: Settings2,
		label: () => "General",
		render: (titleHidden) => <GeneralSettingsSection titleHidden={titleHidden} />,
	},
	{
		id: "harness",
		icon: Bot,
		label: () => "Harness",
		render: (titleHidden) => <HarnessSettingsSection titleHidden={titleHidden} />,
	},
	{
		id: "browserProfiles",
		icon: Globe2,
		label: () => "Browser",
		render: (titleHidden) => (
			<>
				<BrowserProfilesSection titleHidden={titleHidden} />
				<div className="border-t border-border/60 pt-5">
					<BrowserDownloadsSection />
				</div>
			</>
		),
	},
	{
		id: "mobile",
		icon: Smartphone,
		label: () => "Mobile",
		disabled: true,
		render: (titleHidden) => (
			<SettingsSection titleHidden={titleHidden} title="Mobile">
				<div className="rounded-md bg-[var(--color-bg-settings-row)] px-3 py-4">
					<p className="text-sm text-settings-label">{"Mobile support is currently unavailable."}</p>
					<p className="mt-1 text-caption text-settings-muted">
						{"The Connect Mobile entry point remains visible for a future release."}
					</p>
				</div>
			</SettingsSection>
		),
	},
	{
		// The user's own opencode config, and the tool policy Open Agents layers
		// on a manager session. Sits after Mobile to match the sidebar order.
		id: "tools",
		icon: Wrench,
		label: () => "Tools",
		render: (titleHidden) => <OpencodeConfigSection titleHidden={titleHidden} />,
	},
	{
		id: "shortcuts",
		icon: Keyboard,
		label: () => "Shortcuts",
		render: ( titleHidden) => (
			<SettingsSection titleHidden={titleHidden} title="Keyboard shortcuts">
				<SettingsContentPanel><KeyboardShortcutsContent active /></SettingsContentPanel>
			</SettingsSection>
		),
	},
	{
		id: "updates",
		icon: RefreshCw,
		label: () => "Updates",
		render: (titleHidden) => <UpdatesSection titleHidden={titleHidden} />,
	},
	{
		id: "help",
		icon: CircleHelp,
		label: () => "Help",
		render: ( titleHidden) => (
			<SettingsSection titleHidden={titleHidden} title="Report a problem">
				<SettingsContentPanel><ReportProblemContent active /></SettingsContentPanel>
			</SettingsSection>
		),
	},
];

export function visibleGlobalSettings(): SettingsCatalogItem[] {
	return globalSettingsCatalog;
}

export function globalSettingsItem(section: GlobalSettingsSection): SettingsCatalogItem {
	return visibleGlobalSettings().find((item) => item.id === section) ?? globalSettingsCatalog[0];
}

export function globalSettingsItemsFor(section: GlobalSettingsSection | "all"): SettingsCatalogItem[] {
	return section === "all" ? visibleGlobalSettings() : [globalSettingsItem(section)];
}
