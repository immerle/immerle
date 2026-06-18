import { ReactNode, useState } from 'react';
import {
  ActivityIndicator,
  Modal,
  Pressable,
  PressableProps,
  Text,
  TextInput,
  TextInputProps,
  View,
} from 'react-native';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';

// --- Button ----------------------------------------------------------------

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
type ButtonSize = 'sm' | 'md' | 'lg';

interface ButtonProps extends PressableProps {
  title: string;
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: string;
  loading?: boolean;
}

// Spotify-style: fully-rounded pills, bold label, a subtle scale on press.
const variantClasses: Record<ButtonVariant, { bg: string; text: string }> = {
  primary: { bg: 'bg-primary active:scale-95', text: 'text-primary-foreground' },
  secondary: { bg: 'border border-border bg-transparent active:scale-95', text: 'text-foreground' },
  ghost: { bg: 'bg-transparent active:opacity-60', text: 'text-muted' },
  danger: { bg: 'bg-danger active:scale-95', text: 'text-white' },
};

const sizeClasses: Record<ButtonSize, { pad: string; text: string; icon: number }> = {
  sm: { pad: 'px-4 py-2', text: 'text-sm', icon: 16 },
  md: { pad: 'px-5 py-3', text: 'text-base', icon: 18 },
  lg: { pad: 'px-7 py-4', text: 'text-base', icon: 20 },
};

export function Button({
  title,
  variant = 'primary',
  size = 'md',
  icon,
  loading,
  disabled,
  ...rest
}: ButtonProps) {
  const v = variantClasses[variant];
  const s = sizeClasses[size];
  return (
    <Pressable
      accessibilityRole="button"
      disabled={disabled || loading}
      className={`flex-row items-center justify-center gap-2 rounded-full ${s.pad} ${v.bg} ${
        disabled || loading ? 'opacity-50' : ''
      }`}
      {...rest}
    >
      {loading ? (
        <ActivityIndicator color={variant === 'primary' ? '#000' : '#fff'} size="small" />
      ) : (
        <>
          {icon ? <IconColor name={icon} variant={variant} size={s.icon} /> : null}
          <Text className={`font-bold tracking-tight ${s.text} ${v.text}`}>{title}</Text>
        </>
      )}
    </Pressable>
  );
}

function IconColor({ name, variant, size }: { name: string; variant: ButtonVariant; size: number }) {
  const colors = useColors();
  const color =
    variant === 'primary'
      ? colors.primaryForeground
      : variant === 'danger'
        ? '#fff'
        : variant === 'ghost'
          ? colors.muted
          : colors.foreground;
  return <Ionicon name={name} size={size} color={color} />;
}

// --- Icon button -----------------------------------------------------------

export function IconButton({
  name,
  size = 24,
  color,
  onPress,
  hitSlop = 12,
  accessibilityLabel,
}: {
  name: string;
  size?: number;
  color?: string;
  onPress?: () => void;
  hitSlop?: number;
  accessibilityLabel?: string;
}) {
  const colors = useColors();
  return (
    <Pressable
      onPress={onPress}
      hitSlop={hitSlop}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      className="active:opacity-60"
    >
      <Ionicon name={name} size={size} color={color ?? colors.foreground} />
    </Pressable>
  );
}

// --- Text field ------------------------------------------------------------

interface FieldProps extends TextInputProps {
  label?: string;
  icon?: string;
  /** Inline validation error; also flags the input as invalid for a11y. */
  error?: string;
  /** Optional helper text shown under the field when there's no error. */
  help?: string;
  /** Trailing accessory (e.g. a show/hide-password button). */
  trailing?: ReactNode;
}

export function Field({ label, icon, error, help, trailing, onFocus, onBlur, ...rest }: FieldProps) {
  const colors = useColors();
  const [focused, setFocused] = useState(false);
  const borderClass = error ? 'border-danger' : focused ? 'border-primary' : 'border-border';
  return (
    <View className="gap-1.5">
      {label ? <Text className="text-sm font-medium text-muted">{label}</Text> : null}
      <View className={`flex-row items-center gap-2 rounded-xl border bg-surface px-3 ${borderClass}`}>
        {icon ? <Ionicon name={icon} size={18} color={colors.muted} /> : null}
        <TextInput
          placeholderTextColor={colors.muted}
          className="flex-1 py-3 text-base text-foreground"
          aria-invalid={!!error}
          onFocus={(e) => {
            setFocused(true);
            onFocus?.(e);
          }}
          onBlur={(e) => {
            setFocused(false);
            onBlur?.(e);
          }}
          {...rest}
        />
        {trailing}
      </View>
      {error ? (
        <Text className="text-xs text-danger" accessibilityLiveRegion="polite" role="alert">
          {error}
        </Text>
      ) : help ? (
        <Text className="text-xs text-muted">{help}</Text>
      ) : null}
    </View>
  );
}

// --- Containers & states ---------------------------------------------------

export function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  // Borderless, elevation-based separation — the Spotify way.
  return <View className={`rounded-xl bg-surface p-4 ${className}`}>{children}</View>;
}

/** Pill filter/selection chip. */
export function Chip({
  label,
  active,
  onPress,
  icon,
}: {
  label: string;
  active?: boolean;
  onPress?: () => void;
  icon?: string;
}) {
  const colors = useColors();
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      className={`flex-row items-center gap-1.5 self-start rounded-full px-4 py-2 active:opacity-80 ${
        active ? 'bg-primary' : 'bg-surface-alt'
      }`}
    >
      {icon ? (
        <Ionicon name={icon} size={15} color={active ? colors.primaryForeground : colors.foreground} />
      ) : null}
      <Text
        className={`text-sm font-semibold ${active ? 'text-primary-foreground' : 'text-foreground'}`}
      >
        {label}
      </Text>
    </Pressable>
  );
}

export function Loading({ label }: { label?: string }) {
  const colors = useColors();
  return (
    <View className="flex-1 items-center justify-center gap-3 p-8">
      <ActivityIndicator color={colors.primary} size="large" />
      {label ? <Text className="text-sm text-muted">{label}</Text> : null}
    </View>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  const colors = useColors();
  return (
    <View className="flex-1 items-center justify-center gap-3 p-8">
      <Ionicon name="cloud-offline-outline" size={42} color={colors.danger} />
      <Text className="text-center text-base text-foreground">{message}</Text>
      {onRetry ? <Button title="Réessayer" variant="secondary" onPress={onRetry} /> : null}
    </View>
  );
}

export function EmptyState({ icon = 'file-tray-outline', title, subtitle }: { icon?: string; title: string; subtitle?: string }) {
  const colors = useColors();
  return (
    <View className="flex-1 items-center justify-center gap-2 p-8">
      <Ionicon name={icon} size={42} color={colors.muted} />
      <Text className="text-center text-base font-semibold text-foreground">{title}</Text>
      {subtitle ? <Text className="text-center text-sm text-muted">{subtitle}</Text> : null}
    </View>
  );
}

export function Badge({ label, tone = 'default' }: { label: string; tone?: 'default' | 'success' | 'danger' | 'primary' }) {
  const bg =
    tone === 'success'
      ? 'bg-success/15'
      : tone === 'danger'
        ? 'bg-danger/15'
        : tone === 'primary'
          ? 'bg-primary/15'
          : 'bg-surface-alt';
  const text =
    tone === 'success'
      ? 'text-success'
      : tone === 'danger'
        ? 'text-danger'
        : tone === 'primary'
          ? 'text-primary'
          : 'text-muted';
  return (
    <View className={`self-start rounded-full px-2 py-0.5 ${bg}`}>
      <Text className={`text-xs font-medium ${text}`}>{label}</Text>
    </View>
  );
}

export function SectionHeader({ title, action }: { title: string; action?: ReactNode }) {
  return (
    <View className="flex-row items-center justify-between px-4 pb-2 pt-6">
      <Text className="text-xl font-bold tracking-tight text-foreground">{title}</Text>
      {action}
    </View>
  );
}

// --- Select ----------------------------------------------------------------

/** Dependency-free dropdown: a trigger that opens a modal list of options. */
export function Select<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
}) {
  const colors = useColors();
  const [open, setOpen] = useState(false);
  const current = options.find((o) => o.value === value);
  return (
    <>
      <Pressable
        onPress={() => setOpen(true)}
        className="flex-row items-center justify-between rounded-xl border border-border bg-surface-alt px-3 py-2.5 active:opacity-70"
      >
        <Text className="text-base text-foreground">{current?.label ?? value}</Text>
        <Ionicon name="chevron-down" size={18} color={colors.muted} />
      </Pressable>
      <Modal visible={open} transparent animationType="fade" onRequestClose={() => setOpen(false)}>
        <Pressable className="flex-1 justify-center bg-black/40 px-8" onPress={() => setOpen(false)}>
          <View className="overflow-hidden rounded-2xl border border-border bg-surface">
            {options.map((o, i) => {
              const active = o.value === value;
              return (
                <Pressable
                  key={o.value}
                  onPress={() => {
                    onChange(o.value);
                    setOpen(false);
                  }}
                  className={`flex-row items-center justify-between px-4 py-3 ${i > 0 ? 'border-t border-border' : ''} ${active ? 'bg-primary/10' : 'active:bg-surface-alt'}`}
                >
                  <Text className={`text-base ${active ? 'font-semibold text-primary' : 'text-foreground'}`}>{o.label}</Text>
                  {active ? <Ionicon name="checkmark" size={18} color={colors.primary} /> : null}
                </Pressable>
              );
            })}
          </View>
        </Pressable>
      </Modal>
    </>
  );
}
