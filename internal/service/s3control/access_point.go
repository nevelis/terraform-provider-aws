package s3control

import (
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/s3control"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
)

func ResourceAccessPoint() *schema.Resource {
	return &schema.Resource{
		Create: resourceAccessPointCreate,
		Read:   resourceAccessPointRead,
		Update: resourceAccessPointUpdate,
		Delete: resourceAccessPointDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"account_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: verify.ValidAccountID,
			},
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},
			"domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"has_public_access_policy": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},
			"network_origin": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"policy": {
				Type:             schema.TypeString,
				Optional:         true,
				DiffSuppressFunc: verify.SuppressEquivalentPolicyDiffs,
			},
			"public_access_block_configuration": {
				Type:             schema.TypeList,
				Optional:         true,
				ForceNew:         true,
				MinItems:         0,
				MaxItems:         1,
				DiffSuppressFunc: verify.SuppressMissingOptionalConfigurationBlock,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"block_public_acls": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},
						"block_public_policy": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},
						"ignore_public_acls": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},
						"restrict_public_buckets": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},
					},
				},
			},
			"vpc_configuration": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				MinItems: 0,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"vpc_id": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
		},
	}
}

func resourceAccessPointCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).S3ControlConn

	accountId := meta.(*conns.AWSClient).AccountID
	if v, ok := d.GetOk("account_id"); ok {
		accountId = v.(string)
	}
	name := d.Get("name").(string)

	input := &s3control.CreateAccessPointInput{
		AccountId:                      aws.String(accountId),
		Bucket:                         aws.String(d.Get("bucket").(string)),
		Name:                           aws.String(name),
		PublicAccessBlockConfiguration: expandS3AccessPointPublicAccessBlockConfiguration(d.Get("public_access_block_configuration").([]interface{})),
		VpcConfiguration:               expandS3AccessPointVpcConfiguration(d.Get("vpc_configuration").([]interface{})),
	}

	log.Printf("[DEBUG] Creating S3 Access Point: %s", input)
	output, err := conn.CreateAccessPoint(input)

	if err != nil {
		return fmt.Errorf("error creating S3 Control Access Point (%s): %w", name, err)
	}

	if output == nil {
		return fmt.Errorf("error creating S3 Control Access Point (%s): empty response", name)
	}

	parsedARN, err := arn.Parse(aws.StringValue(output.AccessPointArn))

	if err == nil && strings.HasPrefix(parsedARN.Resource, "outpost/") {
		d.SetId(aws.StringValue(output.AccessPointArn))
		name = aws.StringValue(output.AccessPointArn)
	} else {
		d.SetId(fmt.Sprintf("%s:%s", accountId, name))
	}

	if v, ok := d.GetOk("policy"); ok {
		log.Printf("[DEBUG] Putting S3 Access Point policy: %s", d.Id())
		_, err := conn.PutAccessPointPolicy(&s3control.PutAccessPointPolicyInput{
			AccountId: aws.String(accountId),
			Name:      aws.String(name),
			Policy:    aws.String(v.(string)),
		})

		if err != nil {
			return fmt.Errorf("error putting S3 Access Point (%s) policy: %s", d.Id(), err)
		}
	}

	return resourceAccessPointRead(d, meta)
}

func resourceAccessPointRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).S3ControlConn

	accountId, name, err := AccessPointParseID(d.Id())
	if err != nil {
		return err
	}

	output, err := conn.GetAccessPoint(&s3control.GetAccessPointInput{
		AccountId: aws.String(accountId),
		Name:      aws.String(name),
	})

	if !d.IsNewResource() && tfawserr.ErrCodeEquals(err, errCodeNoSuchAccessPoint) {
		log.Printf("[WARN] S3 Access Point (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err != nil {
		return fmt.Errorf("error reading S3 Access Point (%s): %w", d.Id(), err)
	}

	if output == nil {
		return fmt.Errorf("error reading S3 Access Point (%s): empty response", d.Id())
	}

	if strings.HasPrefix(name, "arn:") {
		parsedAccessPointARN, err := arn.Parse(name)

		if err != nil {
			return fmt.Errorf("error parsing S3 Control Access Point ARN (%s): %w", name, err)
		}

		bucketARN := arn.ARN{
			AccountID: parsedAccessPointARN.AccountID,
			Partition: parsedAccessPointARN.Partition,
			Region:    parsedAccessPointARN.Region,
			Resource: strings.Replace(
				parsedAccessPointARN.Resource,
				fmt.Sprintf("accesspoint/%s", aws.StringValue(output.Name)),
				fmt.Sprintf("bucket/%s", aws.StringValue(output.Bucket)),
				1,
			),
			Service: parsedAccessPointARN.Service,
		}

		d.Set("arn", name)
		d.Set("bucket", bucketARN.String())
	} else {
		accessPointARN := arn.ARN{
			AccountID: accountId,
			Partition: meta.(*conns.AWSClient).Partition,
			Region:    meta.(*conns.AWSClient).Region,
			Resource:  fmt.Sprintf("accesspoint/%s", aws.StringValue(output.Name)),
			Service:   "s3",
		}

		d.Set("arn", accessPointARN.String())
		d.Set("bucket", output.Bucket)
	}

	d.Set("account_id", accountId)
	d.Set("domain_name", meta.(*conns.AWSClient).RegionalHostname(fmt.Sprintf("%s-%s.s3-accesspoint", aws.StringValue(output.Name), accountId)))
	d.Set("name", output.Name)
	d.Set("network_origin", output.NetworkOrigin)
	if err := d.Set("public_access_block_configuration", flattenS3AccessPointPublicAccessBlockConfiguration(output.PublicAccessBlockConfiguration)); err != nil {
		return fmt.Errorf("error setting public_access_block_configuration: %s", err)
	}
	if err := d.Set("vpc_configuration", flattenS3AccessPointVpcConfiguration(output.VpcConfiguration)); err != nil {
		return fmt.Errorf("error setting vpc_configuration: %s", err)
	}

	policyOutput, err := conn.GetAccessPointPolicy(&s3control.GetAccessPointPolicyInput{
		AccountId: aws.String(accountId),
		Name:      aws.String(name),
	})

	if tfawserr.ErrMessageContains(err, "NoSuchAccessPointPolicy", "") {
		d.Set("policy", "")
	} else {
		if err != nil {
			return fmt.Errorf("error reading S3 Access Point (%s) policy: %s", d.Id(), err)
		}

		d.Set("policy", policyOutput.Policy)
	}

	// Return early since S3 on Outposts cannot have public policies
	if strings.HasPrefix(name, "arn:") {
		d.Set("has_public_access_policy", false)

		return nil
	}

	policyStatusOutput, err := conn.GetAccessPointPolicyStatus(&s3control.GetAccessPointPolicyStatusInput{
		AccountId: aws.String(accountId),
		Name:      aws.String(name),
	})

	if tfawserr.ErrMessageContains(err, "NoSuchAccessPointPolicy", "") {
		d.Set("has_public_access_policy", false)
	} else {
		if err != nil {
			return fmt.Errorf("error reading S3 Access Point (%s) policy status: %s", d.Id(), err)
		}

		d.Set("has_public_access_policy", policyStatusOutput.PolicyStatus.IsPublic)
	}

	return nil
}

func resourceAccessPointUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).S3ControlConn

	accountId, name, err := AccessPointParseID(d.Id())
	if err != nil {
		return err
	}

	if d.HasChange("policy") {
		if v, ok := d.GetOk("policy"); ok {
			log.Printf("[DEBUG] Putting S3 Access Point policy: %s", d.Id())
			_, err := conn.PutAccessPointPolicy(&s3control.PutAccessPointPolicyInput{
				AccountId: aws.String(accountId),
				Name:      aws.String(name),
				Policy:    aws.String(v.(string)),
			})

			if err != nil {
				return fmt.Errorf("error putting S3 Access Point (%s) policy: %s", d.Id(), err)
			}
		} else {
			log.Printf("[DEBUG] Deleting S3 Access Point policy: %s", d.Id())
			_, err := conn.DeleteAccessPointPolicy(&s3control.DeleteAccessPointPolicyInput{
				AccountId: aws.String(accountId),
				Name:      aws.String(name),
			})

			if err != nil {
				return fmt.Errorf("error deleting S3 Access Point (%s) policy: %s", d.Id(), err)
			}
		}
	}

	return resourceAccessPointRead(d, meta)
}

func resourceAccessPointDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).S3ControlConn

	accountId, name, err := AccessPointParseID(d.Id())
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] Deleting S3 Access Point: %s", d.Id())
	_, err = conn.DeleteAccessPoint(&s3control.DeleteAccessPointInput{
		AccountId: aws.String(accountId),
		Name:      aws.String(name),
	})

	if tfawserr.ErrMessageContains(err, "NoSuchAccessPoint", "") {
		return nil
	}

	if err != nil {
		return fmt.Errorf("error deleting S3 Access Point (%s): %s", d.Id(), err)
	}

	return nil
}

// AccessPointParseID returns the Account ID and Access Point Name (S3) or ARN (S3 on Outposts)
func AccessPointParseID(id string) (string, string, error) {
	parsedARN, err := arn.Parse(id)

	if err == nil {
		return parsedARN.AccountID, id, nil
	}

	parts := strings.SplitN(id, ":", 2)

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected format of ID (%s), expected ACCOUNT_ID:NAME", id)
	}

	return parts[0], parts[1], nil
}

func expandS3AccessPointVpcConfiguration(vConfig []interface{}) *s3control.VpcConfiguration {
	if len(vConfig) == 0 || vConfig[0] == nil {
		return nil
	}

	mConfig := vConfig[0].(map[string]interface{})

	return &s3control.VpcConfiguration{
		VpcId: aws.String(mConfig["vpc_id"].(string)),
	}
}

func flattenS3AccessPointVpcConfiguration(config *s3control.VpcConfiguration) []interface{} {
	if config == nil {
		return []interface{}{}
	}

	return []interface{}{map[string]interface{}{
		"vpc_id": aws.StringValue(config.VpcId),
	}}
}

func expandS3AccessPointPublicAccessBlockConfiguration(vConfig []interface{}) *s3control.PublicAccessBlockConfiguration {
	if len(vConfig) == 0 || vConfig[0] == nil {
		return nil
	}

	mConfig := vConfig[0].(map[string]interface{})

	return &s3control.PublicAccessBlockConfiguration{
		BlockPublicAcls:       aws.Bool(mConfig["block_public_acls"].(bool)),
		BlockPublicPolicy:     aws.Bool(mConfig["block_public_policy"].(bool)),
		IgnorePublicAcls:      aws.Bool(mConfig["ignore_public_acls"].(bool)),
		RestrictPublicBuckets: aws.Bool(mConfig["restrict_public_buckets"].(bool)),
	}
}

func flattenS3AccessPointPublicAccessBlockConfiguration(config *s3control.PublicAccessBlockConfiguration) []interface{} {
	if config == nil {
		return []interface{}{}
	}

	return []interface{}{map[string]interface{}{
		"block_public_acls":       aws.BoolValue(config.BlockPublicAcls),
		"block_public_policy":     aws.BoolValue(config.BlockPublicPolicy),
		"ignore_public_acls":      aws.BoolValue(config.IgnorePublicAcls),
		"restrict_public_buckets": aws.BoolValue(config.RestrictPublicBuckets),
	}}
}
