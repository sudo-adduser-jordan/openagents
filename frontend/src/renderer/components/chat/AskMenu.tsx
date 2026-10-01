import { useCallback, useRef, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { Check } from "lucide-react";
import { cn } from "../../lib/utils";

export interface AskMenuOption {
	value: string;
	label: string;
	description?: string;
}

/**
 * Inline ask menu for one agent question.
 *
 * The opencode question UI is a paused turn with the question text and its
 * options as a single shared menu: arrow keys move, digits quick-pick, Enter
 * confirms. This mirrors that inline rather than using the dropdown
 * OptionMenu portal — the visual treatment (one shared surface, quiet
 * dividers, selected fill) follows the same option-menu row language and the
 * composer suggest menu's listbox pattern.
 */
export function AskMenu({
	name,
	label,
	options,
	value,
	multi = false,
	disabled = false,
	invalid = false,
	onSelect,
}: {
	name: string;
	label: string;
	options: AskMenuOption[];
	value: string | string[] | undefined;
	multi?: boolean;
	disabled?: boolean;
	invalid?: boolean;
	onSelect: (value: string) => void;
}) {
	const listRef = useRef<HTMLDivElement>(null);

	const isSelected = useCallback(
		(optionValue: string) =>
			multi
				? Array.isArray(value) && value.includes(optionValue)
				: value === optionValue,
		[multi, value],
	);

	const focusOption = useCallback((index: number) => {
		const buttons = listRef.current?.querySelectorAll<HTMLButtonElement>(
			"[data-ask-option]:not(:disabled)",
		);
		buttons?.[index]?.focus();
	}, []);

	const focusRelative = useCallback((current: HTMLElement, delta: number) => {
		const buttons = Array.from(
			listRef.current?.querySelectorAll<HTMLButtonElement>(
				"[data-ask-option]:not(:disabled)",
			) ?? [],
		);
		const at = buttons.indexOf(current as HTMLButtonElement);
		if (at < 0 || buttons.length === 0) return;
		const next = (at + delta + buttons.length) % buttons.length;
		buttons[next]?.focus();
	}, []);

	function onKeyDown(event: ReactKeyboardEvent) {
		if (disabled) return;
		const target = event.target;
		const onOption =
			target instanceof HTMLElement && Boolean(target.closest("[data-ask-option]"));
		if (event.key === "ArrowDown" || event.key === "ArrowUp") {
			event.preventDefault();
			if (onOption && target instanceof HTMLElement) {
				focusRelative(target, event.key === "ArrowDown" ? 1 : -1);
			} else {
				focusOption(event.key === "ArrowDown" ? 0 : options.length - 1);
			}
			return;
		}
		if (event.key === "Home" || event.key === "End") {
			event.preventDefault();
			focusOption(event.key === "Home" ? 0 : options.length - 1);
			return;
		}
		if (/^[1-9]$/.test(event.key) && onOption) {
			const index = Number(event.key) - 1;
			const option = options[index];
			if (option) {
				event.preventDefault();
				onSelect(option.value);
			}
		}
	}

	if (options.length === 0) {
		return (
			<p className="rounded-lg border border-border bg-background/50 px-3 py-2.5 text-xs text-muted-foreground">
				No options were offered for this question.
			</p>
		);
	}

	return (
		<div
			ref={listRef}
			role={multi ? "group" : "radiogroup"}
			aria-label={label}
			aria-invalid={invalid || undefined}
			onKeyDown={onKeyDown}
			className="overflow-hidden rounded-lg border border-border bg-background/50"
		>
			{options.map((option, index) => {
				const selected = isSelected(option.value);
				return (
					<button
						key={option.value}
						type="button"
						data-ask-option=""
						role={multi ? "checkbox" : "radio"}
						aria-checked={selected}
						aria-label={`${option.label}${option.description ? `, ${option.description}` : ""}`}
						disabled={disabled}
						onClick={() => onSelect(option.value)}
						className={cn(
							"flex w-full items-start gap-2.5 border-t border-border/60 px-3 py-2 text-left outline-none transition-none first:border-t-0",
							"focus-visible:bg-interactive-active focus-visible:text-foreground",
							selected
								? "bg-interactive-active text-foreground"
								: "bg-transparent text-muted-foreground hover:bg-interactive-hover hover:text-foreground",
							"disabled:pointer-events-none disabled:opacity-50",
						)}
					>
						{options.length <= 9 ? (
							<span
								aria-hidden="true"
								className="mt-px w-3 shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground/70"
							>
								{index + 1}
							</span>
						) : null}
						<span className="min-w-0 flex-1">
							<span className="block text-xs leading-snug text-foreground">
								{option.label}
							</span>
							{option.description ? (
								<span className="mt-0.5 block text-[11px] leading-relaxed text-muted-foreground">
									{option.description}
								</span>
							) : null}
						</span>
						<span className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center">
							{selected ? (
								<Check aria-hidden="true" className="size-3.5 text-foreground" />
							) : (
								<span
									aria-hidden="true"
									className="size-3.5 rounded-full border border-border-strong"
								/>
							)}
						</span>
						<span className="sr-only">{name}</span>
					</button>
				);
			})}
		</div>
	);
}
