The github workflow will build the app then create a docker image which is uploaded to Amazon ECR.
The following trust must be modified and applied through your AWS account to define a OIDC provider
and permit you github repo(s) access.

```json lines
{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Principal": {
				"Federated": "arn:aws:iam::*****:oidc-provider/token.actions.githubusercontent.com"
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": {
				"StringEquals": {
					"token.actions.githubusercontent.com:aud": "https://github.com/XXXXX"
				}
			}
		}
	]
}
```