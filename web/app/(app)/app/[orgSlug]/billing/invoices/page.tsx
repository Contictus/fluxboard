'use client';

import { useQuery } from '@tanstack/react-query';
import { FileText, ExternalLink } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { listInvoices } from '@/lib/api/billing';
import { formatMoney } from '@/lib/billing/format';

const STATUS_STYLE: Record<string, string> = {
  paid: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
  open: 'bg-sky-500/15 text-sky-600 dark:text-sky-400',
  void: 'bg-secondary text-muted-foreground',
  uncollectible: 'bg-destructive/15 text-destructive',
  draft: 'bg-secondary text-muted-foreground',
};

function fmtDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

export default function InvoicesPage() {
  const { orgId } = useOrg();

  const invoices = useQuery({ queryKey: ['invoices', orgId], queryFn: () => listInvoices(orgId) });

  if (invoices.isLoading) {
    return <p className="text-sm text-muted-foreground">Loading invoices…</p>;
  }
  if (invoices.isError) {
    return <p className="text-sm text-destructive">Couldn’t load invoices.</p>;
  }
  if (!invoices.data || invoices.data.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
        No invoices yet. They’ll appear here once your first billing period closes.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-md border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-secondary/30 text-left text-xs text-muted-foreground">
            <th className="px-4 py-2 font-medium">Invoice</th>
            <th className="px-4 py-2 font-medium">Date</th>
            <th className="px-4 py-2 font-medium">Status</th>
            <th className="px-4 py-2 text-right font-medium">Amount</th>
            <th className="px-4 py-2" />
          </tr>
        </thead>
        <tbody>
          {invoices.data.map((inv) => (
            <tr key={inv.id} className="border-b last:border-0">
              <td className="px-4 py-3">
                <span className="flex items-center gap-2">
                  <FileText className="h-4 w-4 text-muted-foreground" />
                  {inv.number || inv.stripe_invoice_id}
                </span>
              </td>
              <td className="px-4 py-3 text-muted-foreground">{fmtDate(inv.created_at)}</td>
              <td className="px-4 py-3">
                <span
                  className={`rounded-full px-2 py-0.5 text-xs font-medium capitalize ${
                    STATUS_STYLE[inv.status] ?? 'bg-secondary text-muted-foreground'
                  }`}
                >
                  {inv.status}
                </span>
              </td>
              <td className="px-4 py-3 text-right font-medium">
                {formatMoney(inv.amount_due, inv.currency)}
              </td>
              <td className="px-4 py-3 text-right">
                {inv.hosted_pdf_url ? (
                  <a
                    href={inv.hosted_pdf_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
                  >
                    PDF <ExternalLink className="h-3.5 w-3.5" />
                  </a>
                ) : null}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
