import React, {useEffect, useState} from 'react';
import CodeBlock from '@theme/CodeBlock';

function randomPassword(): string {
  const bytes = new Uint8Array(18);
  window.crypto.getRandomValues(bytes);
  return btoa(String.fromCharCode(...bytes));
}

const compose = (adminPassword: string) => `services:
  immerle:
    image: ghcr.io/immerle/immerle:latest
    ports:
      - "4533:4533"
    environment:
      DATABASE_DRIVER: "postgres"
      DATABASE_DSN: "postgres://immerle:immerle@postgres:5432/immerle?sslmode=disable"
      LIBRARY_DATA_DIR: "/data"
      LIBRARY_PATHS: "/music"
      ADMIN_USERNAME: "admin"
      ADMIN_PASSWORD: "${adminPassword}"
    volumes:
      - immerle-music:/music:ro
      - immerle-data:/data
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: immerle
      POSTGRES_PASSWORD: immerle
      POSTGRES_DB: immerle
    volumes:
      - immerle-pg:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U immerle"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped

  # Daily pg_dump, keeps the last 7 and prunes anything older.
  backup:
    image: prodrigestivill/postgres-backup-local:16
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      POSTGRES_HOST: postgres
      POSTGRES_DB: immerle
      POSTGRES_USER: immerle
      POSTGRES_PASSWORD: immerle
      SCHEDULE: "@daily"
      BACKUP_KEEP_DAYS: 7
      BACKUP_KEEP_WEEKS: 0
      BACKUP_KEEP_MONTHS: 0
    volumes:
      - immerle_backup:/backups
    restart: unless-stopped

volumes:
  immerle-data:
  immerle-pg:
  immerle-music:
  immerle_backup:
`;

// A fresh admin password baked in every time this component mounts, i.e.
// every page load, so nobody ships "change-me" to production.
export default function QuickStartCompose(): JSX.Element {
  const [password, setPassword] = useState('change-me');
  useEffect(() => setPassword(randomPassword()), []);

  return (
    <CodeBlock language="yaml" title="docker-compose.yml">
      {compose(password)}
    </CodeBlock>
  );
}
