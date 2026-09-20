import { isMacPlatform } from "../../lib/platform";
import { ShortcutBindingsEditor } from "./ShortcutBindingsEditor";

export function KeyboardShortcutsContent({
	active,
	isMac = isMacPlatform(),
}: {
	active: boolean;
	isMac?: boolean;
}) {
	return <ShortcutBindingsEditor mode="content" active={active} isMac={isMac} />;
}