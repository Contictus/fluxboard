'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FormError } from '@/components/auth/field';
import { useToast } from '@/components/ui/toast';
import { ApiError } from '@/lib/api/client';
import { useAuth } from '@/lib/auth/context';
import {
  changePassword,
  enroll2FA,
  activate2FA,
  disable2FA,
} from '@/lib/api/auth';
import { getMe } from '@/lib/api/user';
import type { Enroll2FAResponse } from '@/lib/api/types';

function PasswordCard() {
  const router = useRouter();
  const { logout } = useAuth();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');

  const mutation = useMutation({
    mutationFn: () => changePassword({ current_password: current, new_password: next }),
    // Changing the password revokes every session server-side, so we must re-auth.
    onSuccess: async () => {
      await logout();
      router.replace('/login');
    },
  });

  const err = mutation.error;
  const message =
    err instanceof ApiError
      ? err.status === 401 || err.status === 422
        ? 'Current password is incorrect or the new one is too weak.'
        : err.message
      : err
        ? 'Something went wrong.'
        : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Password</CardTitle>
        <CardDescription>Changing your password signs out all sessions.</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
        >
          <FormError message={message} />
          <Field id="current" label="Current password">
            <Input
              id="current"
              type="password"
              autoComplete="current-password"
              required
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          </Field>
          <Field id="next" label="New password">
            <Input
              id="next"
              type="password"
              autoComplete="new-password"
              required
              minLength={8}
              value={next}
              onChange={(e) => setNext(e.target.value)}
            />
          </Field>
          <Button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? 'Updating…' : 'Change password'}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function TwoFactorCard({ enabled }: { enabled: boolean }) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [enrollment, setEnrollment] = useState<Enroll2FAResponse | null>(null);
  const [code, setCode] = useState('');
  const [recovery, setRecovery] = useState<string[] | null>(null);
  const [disableCode, setDisableCode] = useState('');

  const startEnroll = useMutation({
    mutationFn: enroll2FA,
    onSuccess: (res) => setEnrollment(res),
  });

  const confirm = useMutation({
    mutationFn: () => activate2FA(code),
    onSuccess: async (res) => {
      setRecovery(res.recovery_codes);
      setEnrollment(null);
      setCode('');
      await queryClient.invalidateQueries({ queryKey: ['me'] });
    },
    onError: () => toast({ title: 'Invalid code', variant: 'error' }),
  });

  const turnOff = useMutation({
    mutationFn: () => disable2FA(disableCode),
    onSuccess: async () => {
      setDisableCode('');
      toast({ title: 'Two-factor disabled', variant: 'success' });
      await queryClient.invalidateQueries({ queryKey: ['me'] });
    },
    onError: () => toast({ title: 'Invalid code', variant: 'error' }),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>Two-factor authentication</CardTitle>
        <CardDescription>
          {enabled ? 'Enabled — required at every sign-in.' : 'Add a TOTP authenticator app.'}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {recovery ? (
          <div className="space-y-2 rounded-md border border-primary/30 bg-primary/5 p-4">
            <p className="text-sm font-medium">Save your recovery codes</p>
            <p className="text-xs text-muted-foreground">
              Each code works once if you lose your device. They won&apos;t be shown again.
            </p>
            <ul className="grid grid-cols-2 gap-1 font-mono text-sm">
              {recovery.map((c) => (
                <li key={c}>{c}</li>
              ))}
            </ul>
          </div>
        ) : null}

        {!enabled && !enrollment ? (
          <Button onClick={() => startEnroll.mutate()} disabled={startEnroll.isPending}>
            {startEnroll.isPending ? 'Preparing…' : 'Enable 2FA'}
          </Button>
        ) : null}

        {!enabled && enrollment ? (
          <div className="space-y-3">
            <p className="text-sm">
              Add this secret to your authenticator app, then enter the 6-digit code.
            </p>
            <code className="block break-all rounded-md bg-secondary p-2 font-mono text-sm">
              {enrollment.secret}
            </code>
            <p className="break-all text-xs text-muted-foreground">{enrollment.provisioning_uri}</p>
            <form
              className="flex gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                confirm.mutate();
              }}
            >
              <Input
                inputMode="numeric"
                placeholder="123456"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                required
              />
              <Button type="submit" disabled={confirm.isPending}>
                Verify
              </Button>
            </form>
          </div>
        ) : null}

        {enabled ? (
          <form
            className="flex items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              turnOff.mutate();
            }}
          >
            <Field id="disable" label="Enter a code to disable">
              <Input
                id="disable"
                inputMode="numeric"
                placeholder="123456 or recovery code"
                value={disableCode}
                onChange={(e) => setDisableCode(e.target.value)}
                required
              />
            </Field>
            <Button type="submit" variant="destructive" disabled={turnOff.isPending}>
              Disable
            </Button>
          </form>
        ) : null}
      </CardContent>
    </Card>
  );
}

export default function SecurityPage() {
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe });

  return (
    <div className="space-y-6">
      <PasswordCard />
      <TwoFactorCard enabled={Boolean(me?.totp_enabled)} />
    </div>
  );
}
