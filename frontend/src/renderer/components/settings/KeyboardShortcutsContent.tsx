import { ShortcutBindingsEditor } from "./ShortcutBindingsEditor";

export function KeyboardShortcutsContent({ active }: { active: boolean }) {
	return <ShortcutBindingsEditor mode="content" active={active} />;
}