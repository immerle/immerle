import { ReactNode } from 'react';
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { LinearGradient } from 'expo-linear-gradient';
import { Image } from 'expo-image';
import { useColors } from '../theme/colors';

/**
 * Shared chrome for the auth screens (login / setup): an immersive accent
 * gradient backdrop, the Immerle brand mark, and a floating surface card that
 * holds the form. Centered with a max width so it looks good on desktop too.
 */
export function AuthShell({
  subtitle,
  children,
  footer,
}: {
  subtitle?: string;
  children: ReactNode;
  footer?: ReactNode;
}) {
  const colors = useColors();
  return (
    <View className="flex-1 bg-background">
      {/* Accent glow fading into the page background. */}
      <LinearGradient
        colors={[colors.primary + '55', colors.primary + '14', 'transparent']}
        start={{ x: 0, y: 0 }}
        end={{ x: 0, y: 1 }}
        style={[StyleSheet.absoluteFill, { bottom: undefined, height: 460 }]}
      />
      <SafeAreaView className="flex-1">
        <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1">
          <ScrollView
            contentContainerStyle={{ flexGrow: 1, justifyContent: 'center', padding: 24 }}
            keyboardShouldPersistTaps="handled"
          >
            <View style={{ width: '100%', maxWidth: 420, alignSelf: 'center' }} className="gap-7">
              <Brand subtitle={subtitle} />
              <View
                className="gap-4 rounded-3xl bg-surface p-6"
                style={{
                  shadowColor: '#000',
                  shadowOpacity: 0.25,
                  shadowRadius: 24,
                  shadowOffset: { width: 0, height: 12 },
                  elevation: 8,
                }}
              >
                {children}
              </View>
              {footer ? <View className="items-center gap-3">{footer}</View> : null}
            </View>
          </ScrollView>
        </KeyboardAvoidingView>
      </SafeAreaView>
    </View>
  );
}

function Brand({ subtitle }: { subtitle?: string }) {
  return (
    <View className="items-center gap-3">
      <Image
        source={require('../../assets/logo.png')}
        style={{ width: 100, height: 81 }}
        contentFit="contain"
        accessibilityLabel="Immerle"
      />
      <Text className="text-3xl font-bold tracking-tight text-foreground">Immerle</Text>
      {subtitle ? <Text className="max-w-[300px] text-center text-base text-muted">{subtitle}</Text> : null}
    </View>
  );
}
