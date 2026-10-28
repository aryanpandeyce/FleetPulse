output "firmware_bucket_id" {
  value       = aws_s3_bucket.firmware.id
  description = "Firmware S3 bucket name"
}

output "telemetry_archive_bucket_id" {
  value       = aws_s3_bucket.telemetry_archive.id
  description = "Telemetry archive S3 bucket name"
}

output "workflow_events_queue_url" {
  value       = aws_sqs_queue.workflow_events.url
  description = "Primary workflow SQS queue URL"
}

output "workflow_dlq_url" {
  value       = aws_sqs_queue.workflow_dlq.url
  description = "Workflow dead-letter queue URL"
}
