import { X509Certificate } from 'node:crypto';
import { isIP } from 'node:net';

const serverAuth = '1.3.6.1.5.5.7.3.1';

/**
 * Accepts the control center's TLS certificate only if the pinned cluster CA
 * signed it directly, it is valid now, it is meant for servers and it covers
 * the host. Everything else is rejected, including public CAs: the shell
 * talks to nothing but its own control center.
 */
export function verifyServerCertificate(leafPem: string, caPem: string, host: string, now: Date): boolean {
  try {
    const leaf = new X509Certificate(leafPem);
    const ca = new X509Certificate(caPem);
    if (!ca.ca || !leaf.checkIssued(ca) || !leaf.verify(ca.publicKey)) return false;
    if (now < new Date(leaf.validFrom) || now > new Date(leaf.validTo)) return false;
    // An agent's client certificate comes from the same CA; it must not pass as a server.
    // Node leaves keyUsage undefined when the certificate has no extended key usage.
    const extendedKeyUsage = leaf.keyUsage as readonly string[] | undefined;
    if (!(extendedKeyUsage ?? []).includes(serverAuth)) return false;
    const h = host.replace(/^\[|\]$/g, '');
    return isIP(h) !== 0 ? leaf.checkIP(h) !== undefined : leaf.checkHost(h) !== undefined;
  } catch {
    return false;
  }
}
