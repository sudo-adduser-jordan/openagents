import { isMacPlatform } from "../../lib/platform";
import { ShortcutBindingsEditor } from "./ShortcutBindingsEditor";

export function KeyboardShortcutsSettingsDialog({
	open,
	onOpenChange,
	isMac = isMacPlatform(),
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	isMac?: boolean;
}) {
	return <ShortcutBindingsEditor mode="dialog" open={open} onOpenChange={onOpenChange} isMac={isMac} />;
}