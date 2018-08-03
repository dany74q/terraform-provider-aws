package aws

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
)

func resourceAwsLbListener() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsLbListenerCreate,
		Read:   resourceAwsLbListenerRead,
		Update: resourceAwsLbListenerUpdate,
		Delete: resourceAwsLbListenerDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"load_balancer_arn": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"port": {
				Type:         schema.TypeInt,
				Required:     true,
				ValidateFunc: validation.IntBetween(1, 65535),
			},

			"protocol": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "HTTP",
				StateFunc: func(v interface{}) string {
					return strings.ToUpper(v.(string))
				},
				ValidateFunc: validation.StringInSlice([]string{
					elbv2.ProtocolEnumHttp,
					elbv2.ProtocolEnumHttps,
					elbv2.ProtocolEnumTcp,
				}, true),
			},

			"ssl_policy": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"certificate_arn": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"default_action": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"target_group_arn": {
							Type:     schema.TypeString,
							Required: true,
						},
						"type": {
							Type:     schema.TypeString,
							Required: true,
							ValidateFunc: validation.StringInSlice([]string{
								elbv2.ActionTypeEnumForward,
							}, true),
						},
					},
				},
			},
		},
	}
}

func resourceAwsLbListenerCreate(d *schema.ResourceData, meta interface{}) error {
	elbconn := meta.(*AWSClient).elbv2conn

	lbArn := d.Get("load_balancer_arn").(string)

	params := &elbv2.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Port:            aws.Int64(int64(d.Get("port").(int))),
		Protocol:        aws.String(d.Get("protocol").(string)),
	}

	if sslPolicy, ok := d.GetOk("ssl_policy"); ok {
		params.SslPolicy = aws.String(sslPolicy.(string))
	}

	if certificateArn, ok := d.GetOk("certificate_arn"); ok {
		params.Certificates = make([]*elbv2.Certificate, 1)
		params.Certificates[0] = &elbv2.Certificate{
			CertificateArn: aws.String(certificateArn.(string)),
		}
	}

	if defaultActions := d.Get("default_action").([]interface{}); len(defaultActions) == 1 {
		params.DefaultActions = make([]*elbv2.Action, len(defaultActions))

		for i, defaultAction := range defaultActions {
			defaultActionMap := defaultAction.(map[string]interface{})

			params.DefaultActions[i] = &elbv2.Action{
				TargetGroupArn: aws.String(defaultActionMap["target_group_arn"].(string)),
				Type:           aws.String(defaultActionMap["type"].(string)),
			}
		}
	}

	var resp *elbv2.CreateListenerOutput

	err := resource.Retry(5*time.Minute, func() *resource.RetryError {
		var err error
		log.Printf("[DEBUG] Creating LB listener for ARN: %s", d.Get("load_balancer_arn").(string))
		resp, err = elbconn.CreateListener(params)
		if err != nil {
			if isAWSErr(err, elbv2.ErrCodeCertificateNotFoundException, "") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("Error creating LB Listener: %s", err)
	}

	if len(resp.Listeners) == 0 {
		return errors.New("Error creating LB Listener: no listeners returned in response")
	}

	d.SetId(*resp.Listeners[0].ListenerArn)

	// Ensure that the listener is available from the describe call, since AWS may not return it right away.
	log.Printf("[DEBUG] Waiting for the LB Listener (%s) to exist", d.Id())
	stateConf := &resource.StateChangeConf{
		Pending: []string{""},
		Target:  []string{"exists"},
		Refresh: resourceAwsLbListenerRefreshFunc(elbconn, d.Id()),
		Timeout: 3 * time.Minute,
	}
	lbRaw, err := stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(
			"Error waiting for LB Listener (%s) to exist", d.Id())
	}

	log.Printf("[DEBUG] LB Listener (%s) exists", d.Id())
	resourceAwsLbListenerReadData(d, lbRaw.(*elbv2.Listener), meta)
	return nil
}

func resourceAwsLbListenerRead(d *schema.ResourceData, meta interface{}) error {
	elbconn := meta.(*AWSClient).elbv2conn
	lbRaw, _, err := resourceAwsLbListenerRefreshFunc(elbconn, d.Id())()
	if err != nil {
		if isAWSErr(err, elbv2.ErrCodeListenerNotFoundException, "") {
			log.Printf("[WARN] DescribeListeners - removing %s from state", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error retrieving Listener: %s", err)
	}
	if lbRaw == nil {
		log.Printf("[WARN] DescribeListeners - removing %s from state", d.Id())
		d.SetId("")
		return nil
	}

	resourceAwsLbListenerReadData(d, lbRaw.(*elbv2.Listener), meta)
	return nil
}

func resourceAwsLbListenerReadData(d *schema.ResourceData, listener *elbv2.Listener, meta interface{}) {
	d.Set("arn", listener.ListenerArn)
	d.Set("load_balancer_arn", listener.LoadBalancerArn)
	d.Set("port", listener.Port)
	d.Set("protocol", listener.Protocol)
	d.Set("ssl_policy", listener.SslPolicy)

	if listener.Certificates != nil && len(listener.Certificates) == 1 && listener.Certificates[0] != nil {
		d.Set("certificate_arn", listener.Certificates[0].CertificateArn)
	}

	defaultActions := make([]map[string]interface{}, 0)
	if listener.DefaultActions != nil && len(listener.DefaultActions) > 0 {
		for _, defaultAction := range listener.DefaultActions {
			action := map[string]interface{}{
				"target_group_arn": aws.StringValue(defaultAction.TargetGroupArn),
				"type":             aws.StringValue(defaultAction.Type),
			}
			defaultActions = append(defaultActions, action)
		}
	}
	d.Set("default_action", defaultActions)
}

func resourceAwsLbListenerRefreshFunc(conn *elbv2.ELBV2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		resp, err := conn.DescribeListeners(&elbv2.DescribeListenersInput{
			ListenerArns: []*string{aws.String(id)},
		})
		if err != nil {
			if isAWSErr(err, elbv2.ErrCodeListenerNotFoundException, "") {
				resp = nil
				err = nil
			} else {
				return nil, "", fmt.Errorf("Error retrieving Listener: %s", err)
			}
		}

		if resp == nil {
			return nil, "", nil
		}

		if len(resp.Listeners) != 1 {
			return nil, "", fmt.Errorf("Error retrieving Listener %q (expected 1, got %d)",
				id, len(resp.Listeners))
		}

		return resp.Listeners[0], "exists", nil
	}
}

func resourceAwsLbListenerUpdate(d *schema.ResourceData, meta interface{}) error {
	elbconn := meta.(*AWSClient).elbv2conn

	params := &elbv2.ModifyListenerInput{
		ListenerArn: aws.String(d.Id()),
		Port:        aws.Int64(int64(d.Get("port").(int))),
		Protocol:    aws.String(d.Get("protocol").(string)),
	}

	if sslPolicy, ok := d.GetOk("ssl_policy"); ok {
		params.SslPolicy = aws.String(sslPolicy.(string))
	}

	if certificateArn, ok := d.GetOk("certificate_arn"); ok {
		params.Certificates = make([]*elbv2.Certificate, 1)
		params.Certificates[0] = &elbv2.Certificate{
			CertificateArn: aws.String(certificateArn.(string)),
		}
	}

	if defaultActions := d.Get("default_action").([]interface{}); len(defaultActions) == 1 {
		params.DefaultActions = make([]*elbv2.Action, len(defaultActions))

		for i, defaultAction := range defaultActions {
			defaultActionMap := defaultAction.(map[string]interface{})

			params.DefaultActions[i] = &elbv2.Action{
				TargetGroupArn: aws.String(defaultActionMap["target_group_arn"].(string)),
				Type:           aws.String(defaultActionMap["type"].(string)),
			}
		}
	}

	err := resource.Retry(5*time.Minute, func() *resource.RetryError {
		_, err := elbconn.ModifyListener(params)
		if err != nil {
			if isAWSErr(err, elbv2.ErrCodeCertificateNotFoundException, "") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("Error modifying LB Listener: %s", err)
	}

	return resourceAwsLbListenerRead(d, meta)
}

func resourceAwsLbListenerDelete(d *schema.ResourceData, meta interface{}) error {
	elbconn := meta.(*AWSClient).elbv2conn

	_, err := elbconn.DeleteListener(&elbv2.DeleteListenerInput{
		ListenerArn: aws.String(d.Id()),
	})
	if err != nil {
		return fmt.Errorf("Error deleting Listener: %s", err)
	}

	return nil
}
