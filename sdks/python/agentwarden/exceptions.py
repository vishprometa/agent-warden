"""AgentWarden exceptions for governance verdicts."""


class ActionDenied(Exception):
    """Raised when a policy denies or terminates an action.

    Attributes:
        policy_name: The name of the policy that denied the action.
        suggestions: Optional list of suggested alternatives or fixes.
    """

    def __init__(
        self,
        policy_name: str,
        message: str,
        suggestions: list = None,
    ):
        self.policy_name = policy_name
        self.suggestions = suggestions or []
        super().__init__(message)


class ActionPendingApproval(Exception):
    """Raised when an action requires human approval before proceeding.

    Attributes:
        approval_id: The ID of the pending approval request.
        policy_name: The name of the policy that requires approval.
        timeout_seconds: How long the approval window lasts before auto-deny.
    """

    def __init__(
        self,
        approval_id: str,
        policy_name: str,
        timeout_seconds: int,
    ):
        self.approval_id = approval_id
        self.policy_name = policy_name
        self.timeout_seconds = timeout_seconds
        super().__init__(f"Action pending approval: {approval_id}")
