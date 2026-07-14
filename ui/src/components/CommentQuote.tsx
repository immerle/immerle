import { Text, TextStyle, StyleProp } from 'react-native';
import { useAuth } from '../auth/store';

interface CommentQuoteProps {
  comment: string;
  className?: string;
  style?: StyleProp<TextStyle>;
  numberOfLines?: number;
}

/**
 * A personal nostalgia note, formatted as an attributed quote: `"note" — Name`
 * (the current user's display name, falling back to their username).
 */
export function CommentQuote({ comment, className, style, numberOfLines = 1 }: CommentQuoteProps) {
  const displayNameState = useAuth((s) => s.displayName);
  const username = useAuth((s) => s.client?.username);
  const displayName = displayNameState ?? username ?? '';
  return (
    <Text numberOfLines={numberOfLines} className={className} style={style}>
      "{comment}" - {displayName}
    </Text>
  );
}
