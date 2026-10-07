import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.inovarapp.mobile',
  appName: 'InovarApp',
  webDir: 'mobile/www',
  server: {
    hostname: 'localhost',
    androidScheme: 'https',
    iosScheme: 'capacitor',
  },
};

export default config;
