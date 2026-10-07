import { Contacts } from '@capacitor/contacts';
import { App } from '@capacitor/app';
import { Browser } from '@capacitor/browser';
import { Capacitor } from '@capacitor/core';
import { PushNotifications } from '@capacitor/push-notifications';
import { Geolocation } from '@capacitor/geolocation';

declare global {
  interface Window {
    inovarPickDeviceContact?: () => Promise<unknown>;
    inovarNativeOAuthAvailable?: boolean;
    inovarNativeOAuthReady?: Promise<void>;
    inovarOpenOAuth?: (url: string) => Promise<void>;
    inovarPendingOAuthURL?: string;
    inovarPendingOAuthError?: boolean;
    inovarHandleOAuthURL?: (url: string) => void;
    inovarHandleOAuthError?: () => void;
    inovarNativePushAvailable?: boolean;
    inovarNativePushReady?: Promise<void>;
    inovarNativePushToken?: { platform: string; token: string };
    inovarNativePushPermitted?: boolean;
    inovarRequestNativePush?: () => Promise<{ platform: string; token: string }>;
    inovarOnNativePushToken?: (platform: string, token: string) => void;
    inovarGetCurrentLocation?: () => Promise<{ latitude: number; longitude: number; accuracy: number }>;
  }
}

// The Go/WASM UI calls this very small Capacitor bridge. Contact selection,
// permission prompts, and platform-specific behavior remain native plugin work.
window.inovarPickDeviceContact = async () => Contacts.pickContact();

window.inovarGetCurrentLocation = async () => {
  if (Capacitor.isNativePlatform()) {
    const permission = await Geolocation.requestPermissions({ permissions: ['coarseLocation'] });
    if (permission.coarseLocation !== 'granted') throw new Error('Permissão de localização não concedida.');
  }
  const position = await Geolocation.getCurrentPosition({ enableHighAccuracy: false, timeout: 20000, maximumAge: 300000 });
  return {
    latitude: position.coords.latitude,
    longitude: position.coords.longitude,
    accuracy: position.coords.accuracy,
  };
};

window.inovarNativePushAvailable = false;
window.inovarNativePushReady = (async () => {
  if (!Capacitor.isNativePlatform()) return;
  await PushNotifications.addListener('registration', ({ value }) => {
    window.inovarNativePushToken = { platform: Capacitor.getPlatform(), token: value };
    window.inovarNativePushPermitted = true;
    window.inovarOnNativePushToken?.(Capacitor.getPlatform(), value);
  });
  window.inovarRequestNativePush = async () => {
    const permission = await PushNotifications.requestPermissions();
    if (permission.receive !== 'granted') throw new Error('Permissão de notificações não concedida.');
    const token = await new Promise<string>((resolve, reject) => {
      let settled = false;
      void (async () => {
        let registration: Awaited<ReturnType<typeof PushNotifications.addListener>> | undefined;
        let failure: Awaited<ReturnType<typeof PushNotifications.addListener>> | undefined;
        registration = await PushNotifications.addListener('registration', (result) => {
          if (settled) return;
          settled = true;
          void registration?.remove();
          void failure?.remove();
          resolve(result.value);
        });
        failure = await PushNotifications.addListener('registrationError', (error) => {
          if (settled) return;
          settled = true;
          void registration?.remove();
          void failure?.remove();
          reject(new Error(error.error ?? 'Falha no registro nativo de notificações.'));
        });
        await PushNotifications.register();
      })().catch(reject);
    });
    return { platform: Capacitor.getPlatform(), token };
  };
  window.inovarNativePushAvailable = true;
})();

const callbackScheme = 'com.inovarapp.mobile:';
const callbackHost = 'oauth-callback';
let lastCallbackURL = '';

function deliverOAuthURL(url: string) {
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== callbackScheme || parsed.hostname !== callbackHost || url === lastCallbackURL) return;
    lastCallbackURL = url;
    if (parsed.searchParams.has('error') || parsed.searchParams.has('error_code') || parsed.hash.includes('error=')) {
      deliverOAuthError();
      return;
    }
    if (window.inovarHandleOAuthURL) window.inovarHandleOAuthURL(url);
    else window.inovarPendingOAuthURL = url;
  } catch {
    // Ignore unrelated or malformed native URLs.
  }
}

function deliverOAuthError() {
  if (window.inovarHandleOAuthError) window.inovarHandleOAuthError();
  else window.inovarPendingOAuthError = true;
}

window.inovarNativeOAuthAvailable = false;
window.inovarNativeOAuthReady = (async () => {
  if (!Capacitor.isNativePlatform()) return;
  await App.addListener('appUrlOpen', ({ url }) => deliverOAuthURL(url));
  window.inovarOpenOAuth = async (url: string) => {
    try {
      await Browser.open({ url, presentationStyle: 'fullscreen' });
    } catch {
      deliverOAuthError();
    }
  };
  window.inovarNativeOAuthAvailable = true;
  void App.getLaunchUrl().then((launch) => {
    if (launch?.url) deliverOAuthURL(launch.url);
  }).catch(() => undefined);
})();
