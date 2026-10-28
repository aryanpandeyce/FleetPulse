# Firmware Storage Bucket
resource "aws_s3_bucket" "firmware" {
  bucket = "fleetpulse-firmware"

  tags = {
    Environment = var.environment
    Service     = "workflow-service"
    ManagedBy   = "terraform"
  }
}

resource "aws_s3_bucket_versioning" "firmware_versioning" {
  bucket = aws_s3_bucket.firmware.id
  versioning_configuration {
    status = "Enabled"
  }
}

# Long-term Telemetry Archive Bucket
resource "aws_s3_bucket" "telemetry_archive" {
  bucket = "fleetpulse-telemetry-archive"

  tags = {
    Environment = var.environment
    Service     = "telemetry-service"
    ManagedBy   = "terraform"
  }
}
