variable "aws_region" {
  type        = string
  default     = "us-east-1"
  description = "AWS deployment region"
}

variable "aws_access_key" {
  type        = string
  default     = "mock_access_key"
  description = "AWS access key (mocked for LocalStack)"
}

variable "aws_secret_key" {
  type        = string
  default     = "mock_secret_key"
  description = "AWS secret key (mocked for LocalStack)"
}

variable "localstack_endpoint" {
  type        = string
  default     = "http://localhost:4566"
  description = "LocalStack endpoint URL (set to null or empty for real AWS)"
}

variable "environment" {
  type        = string
  default     = "local"
  description = "Environment name (local, staging, prod)"
}
