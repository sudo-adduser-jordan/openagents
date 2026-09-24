import { Fragment, Suspense } from "react";
import type { GlobalSettingsSection } from "../stores/ui-store";
import { globalSettingsItemsFor } from "./settings/settingsCatalog";

export type { GlobalSettingsSection };

export function GlobalSettingsForm({
	section = "all",
}: {
	section?: GlobalSettingsSection | "all";
}) {
	const all = section === "all";
	// One section per page means the dialog header already names it, so a
	// leading in-page heading would just repeat that title.
	const titleHidden = !all;

	return (
		<div
			aria-label="Settings"
			className="flex w-full flex-col gap-(--size-settings-section-gap)"
			data-testid="settings-page"
		>
			{globalSettingsItemsFor(section).map((item) => (
				<Fragment key={item.id}>
					<Suspense fallback={null}>{item.render(titleHidden)}</Suspense>
				</Fragment>
			))}
		</div>
	);
}
