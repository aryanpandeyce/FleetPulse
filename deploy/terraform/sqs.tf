# Workflow Dead-Letter Queue (DLQ)
resource "aws_sqs_queue" "workflow_dlq" {
  name                      = "workflow-dlq"
  message_retention_seconds = 1209600 # 14 days

  tags = {
    Environment = var.environment
    Service     = "workflow-service"
  }
}

# Primary Workflow Event Queue with Redrive Policy
resource "aws_sqs_queue" "workflow_events" {
  name                      = "workflow-events"
  message_retention_seconds = 86400 # 1 day

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.workflow_dlq.arn
    maxReceiveCount     = 3
  })

  tags = {
    Environment = var.environment
    Service     = "workflow-service"
  }
}
