import { Button } from "../ui/button";

// Store listings are intentionally not linked until the new native identities
// are provisioned. Keeping the old listings would route users to the retired
// product, while inventing replacement listing URLs would send them to 404s.
export const IOS_APP_STORE_URL: string | null = null;
export const ANDROID_PLAY_STORE_URL: string | null = null;

const STORES = [
	{ name: "iOS" },
	{ name: "Android" },
] as const;

// ConnectMobileGetApp remains visible before pairing so the unavailable native
// distribution state is explicit rather than looking like a broken QR flow.
export function ConnectMobileGetApp() {
	return (
		<div className="flex flex-col">
			<span className="px-3 py-3 text-subtitle leading-(--leading-settings-mobile-title) text-settings-label">{"Get the app"}</span>
			{STORES.map(({ name }) => (
				<div key={name} className="flex items-center justify-between gap-3 px-3 py-3">
					<div className="flex min-w-0 flex-col">
						<span className="text-sm leading-5 text-settings-label">{name}</span>
						<span className="text-caption leading-(--leading-settings-mobile-hint) text-settings-muted">
							{"The Open Agents store listing is being provisioned."}
						</span>
					</div>
					<Button type="button" variant="footer" className="rounded-md" disabled>
						{"Coming soon"}
					</Button>
				</div>
			))}
		</div>
	);
}
