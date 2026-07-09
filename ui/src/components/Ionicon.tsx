import Ionicons from '@expo/vector-icons/Ionicons';
import { ComponentProps } from 'react';

type IoniconName = ComponentProps<typeof Ionicons>['name'];

interface IoniconProps {
  name: string;
  size?: number;
  color?: ComponentProps<typeof Ionicons>['color'];
}

/**
 * Thin wrapper over Ionicons so call sites can pass plain string names without
 * importing the (very large) icon name union everywhere.
 */
export function Ionicon({ name, size = 20, color }: IoniconProps) {
  return <Ionicons name={name as IoniconName} size={size} color={color} />;
}
