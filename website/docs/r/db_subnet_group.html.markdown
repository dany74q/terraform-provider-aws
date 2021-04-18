---
subcategory: "RDS"
layout: "aws"
page_title: "AWS: aws_db_subnet_group"
description: |-
  Provides an RDS DB subnet group resource.
---

# Resource: aws_db_subnet_group

Provides an RDS DB subnet group resource.

> **Hands-on:** For an example of the `aws_db_subnet_group` in use, follow the [Manage AWS RDS Instances](https://learn.hashicorp.com/tutorials/terraform/aws-rds?in=terraform/aws&utm_source=WEBSITE&utm_medium=WEB_IO&utm_offer=ARTICLE_PAGE&utm_content=DOCS) tutorial on HashiCorp Learn.

## Example Usage

```terraform
resource "aws_db_subnet_group" "default" {
  name       = "main"
  subnet_ids = [aws_subnet.frontend.id, aws_subnet.backend.id]

  tags = {
    Name = "My DB subnet group"
  }
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Optional, Forces new resource) The name of the DB subnet group. If omitted, this provider will assign a random, unique name.
* `name_prefix` - (Optional, Forces new resource) Creates a unique name beginning with the specified prefix. Conflicts with `name`.
* `description` - (Optional) The description of the DB subnet group. Defaults to "Managed by Pulumi".
* `subnet_ids` - (Required) A list of VPC subnet IDs.
* `tags` - (Optional) A map of tags to assign to the resource.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - The db subnet group name.
* `arn` - The ARN of the db subnet group.


## Import

DB Subnet groups can be imported using the `name`, e.g.

```
$ terraform import aws_db_subnet_group.default production-subnet-group
```
