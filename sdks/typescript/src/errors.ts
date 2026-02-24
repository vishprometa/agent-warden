export class ActionDenied extends Error {
  readonly policyName: string;
  readonly suggestions: string[];

  constructor(policyName: string, message: string, suggestions: string[] = []) {
    super(message);
    this.name = 'ActionDenied';
    this.policyName = policyName;
    this.suggestions = suggestions;
  }
}

export class ActionPendingApproval extends Error {
  readonly approvalId: string;
  readonly policyName: string;
  readonly timeoutSeconds: number;

  constructor(approvalId: string, policyName: string, timeoutSeconds: number) {
    super(`Action pending approval: ${approvalId}`);
    this.name = 'ActionPendingApproval';
    this.approvalId = approvalId;
    this.policyName = policyName;
    this.timeoutSeconds = timeoutSeconds;
  }
}
